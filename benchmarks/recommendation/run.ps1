#Requires -Version 5.1
param(
    [string]$BaselineRoot = 'G:/GoCode/GCFeed/apps/api',
    [string]$Config = 'G:/GoCode/DreamReel/apps/api/configs/config.yaml',
    [int]$Videos = 10000,
    [int]$Rounds = 3,
    [string]$Duration = '20s',
    [string]$RunName = (Get-Date -Format 'yyyyMMdd-HHmmss')
)
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$work = Join-Path $PSScriptRoot '.work'
$result = Join-Path $PSScriptRoot ('results/' + $RunName)
New-Item -ItemType Directory -Path $result -Force | Out-Null
$fixture = Join-Path $result 'fixture.json'
$processes = @()
$savedEnvironment = @{}
foreach ($name in @('GOMAXPROCS','MODE','BASE_URL','DURATION','RESULT','VUS','RATE')) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name,'Process')
}
$env:GOMAXPROCS = '4'
$env:VUS = '20'

function Invoke-K6 {
    param([string]$LogPath)
    # Windows PowerShell 将原生命令的 stderr 视为错误记录，退出码仍由调用方判断。
    $savedPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & k6 run --no-color --quiet recommendation.js *> $LogPath
        return $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $savedPreference
    }
}

try {
    & python -X utf8 prepare.py --baseline $BaselineRoot
    if ($LASTEXITCODE -ne 0) { throw '构建失败' }
    & "$work/current.exe" -action prepare -config $Config -fixture $fixture -videos $Videos
    if ($LASTEXITCODE -ne 0) { throw '测试数据准备失败' }
    $resource = Get-Content -LiteralPath $fixture -Raw | ConvertFrom-Json
    foreach ($variant in @('baseline','current')) {
        $port = if ($variant -eq 'baseline') { 18080 } else { 18081 }
        $serverArguments = @('-action','serve','-variant',$variant,'-config',('"' + $Config + '"'),'-fixture',('"' + $fixture + '"'),'-port',"$port")
        $proc = Start-Process -FilePath "$work/$variant.exe" -ArgumentList $serverArguments -WorkingDirectory $work -WindowStyle Hidden -PassThru -RedirectStandardOutput "$result/$variant-server.stdout.log" -RedirectStandardError "$result/$variant-server.stderr.log"
        $processes += $proc
        $ready = $false
        for ($i=0; $i -lt 60; $i++) {
            try { $info = Invoke-RestMethod "http://127.0.0.1:$port/__bench/info"; if ($info.variant -eq $variant -and $info.internal_token -eq ("k6-local-" + $resource.id)) { $ready=$true; break } }
            catch { Start-Sleep -Milliseconds 500 }
        }
        if (!$ready) { throw "$variant 未启动" }
    }
    $hardware = [ordered]@{
        services = (& "$work/current.exe" -action describe -config $Config | Out-String | ConvertFrom-Json)
        k6 = (& k6 version | Out-String).Trim()
        go = (& go version | Out-String).Trim()
        processor = @(Get-CimInstance Win32_Processor | Select-Object Name,NumberOfCores,NumberOfLogicalProcessors)
        memory = @(Get-CimInstance Win32_OperatingSystem | Select-Object TotalVisibleMemorySize)
        baselineHead = (& git -C (Split-Path (Split-Path $BaselineRoot)) rev-parse HEAD | Out-String).Trim()
        currentHead = (& git -C (Split-Path (Split-Path $Config)) rev-parse HEAD | Out-String).Trim()
        gomaxprocs = 4
        databasePool = @{maxOpen=50;maxIdle=10}
        videos = $Videos
        users = 64
        embeddingDimensions = 128
        rounds = $Rounds
        duration = $Duration
        cachedArrivalRate = 100
        firstArrivalRate = 50
        scrollVUs = 20
    }
    $hardware | ConvertTo-Json -Depth 8 | Set-Content "$result/environment.json" -Encoding utf8

    # 每轮交替执行顺序，保留原实现的正确性阈值失败记录。
    foreach ($round in 1..$Rounds) {
        $order = if ($round % 2 -eq 1) { @('baseline','current') } else { @('current','baseline') }
        foreach ($mode in @('cached','first','scroll8')) {
            foreach ($variant in $order) {
                $port = if ($variant -eq 'baseline') { 18080 } else { 18081 }
                $env:RATE = if ($mode -eq 'first') { '50' } else { '100' }
                $env:MODE=$mode
                $env:BASE_URL="http://127.0.0.1:$port"
                $env:DURATION=$Duration
                $env:RESULT="$result/$variant-$mode-r$round.json"
                Write-Output "RUN $variant $mode round=$round"
                $k6Exit = Invoke-K6 -LogPath "$result/$variant-$mode-r$round.log"
                if ($k6Exit -ne 0 -and !($variant -eq 'baseline' -and $k6Exit -eq 99)) { throw "k6 场景失败，请检查 $variant-$mode-r$round.log" }
                $summary = Get-Content -LiteralPath $env:RESULT -Raw | ConvertFrom-Json
                $summary.benchmark | Add-Member -NotePropertyName exit_code -NotePropertyValue $k6Exit -Force
                $summary.benchmark.server = Invoke-RestMethod "http://127.0.0.1:$port/__bench/stats"
                $summary | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath $env:RESULT -Encoding utf8
            }
        }
    }
    $env:MODE='long'; $env:BASE_URL='http://127.0.0.1:18081'; $env:DURATION=$Duration; $env:VUS='10'
    foreach ($round in 1..$Rounds) {
        $env:RESULT="$result/current-long-r$round.json"
        Write-Output "RUN current long round=$round"
        $k6Exit = Invoke-K6 -LogPath "$result/current-long-r$round.log"
        if ($k6Exit -ne 0) { throw '长会话场景失败' }
        $summary = Get-Content -LiteralPath $env:RESULT -Raw | ConvertFrom-Json
        $summary.benchmark.server = Invoke-RestMethod 'http://127.0.0.1:18081/__bench/stats'
        $summary | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath $env:RESULT -Encoding utf8
    }
    Copy-Item -LiteralPath recommendation.js -Destination "$result/load-script.js"
    & python -X utf8 analyze.py $result
    Write-Output "RESULTS $result"
}
finally {
    foreach ($proc in $processes) {
        if (!$proc.HasExited) { Stop-Process -Id $proc.Id -ErrorAction Continue }
    }
    if (Test-Path $fixture) {
        & "$work/current.exe" -action cleanup -config $Config -fixture $fixture
        if ($LASTEXITCODE -ne 0) { Write-Warning '清理未完成，请使用 fixture.json 重试 cleanup' }
        else { 'isolated resources cleaned' | Set-Content "$result/cleanup.txt" }
    }
    foreach ($name in $savedEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($name,$savedEnvironment[$name],'Process')
    }
}
