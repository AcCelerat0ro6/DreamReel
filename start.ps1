# Windows: start the deployed server, or deploy the current working tree.
[CmdletBinding()]
param(
    [ValidateSet('start', 'deploy', 'restart', 'stop', 'status', 'logs', 'public-on', 'public-off', 'public-status')]
    [string]$Action = 'start',
    [string]$Target = 'target-wsl',
    [string]$RemotePath = '/home/accelerator/DreamReel'
)
$ErrorActionPreference = 'Stop'
if ($Target -notmatch '^[a-zA-Z0-9_.@-]+$' -or $Target.StartsWith('-')) {
    throw 'Invalid SSH target.'
}
if ($RemotePath -notmatch '^/[a-zA-Z0-9_./-]+$' -or $RemotePath -match '(^|/)\.\.(/|$)' -or $RemotePath -eq '/') {
    throw 'RemotePath must be an absolute Linux project path without spaces or .. segments.'
}
foreach ($Command in @('ssh', 'scp', 'tar')) {
    if (-not (Get-Command $Command -ErrorAction SilentlyContinue)) { throw "Missing command: $Command" }
}
function Assert-NativeExit([string]$Operation) {
    if ($LASTEXITCODE -ne 0) { throw "$Operation failed (exit $LASTEXITCODE)." }
}

if ($Action -eq 'start') {
    & ssh -o BatchMode=yes $Target "test -f '$RemotePath/start.sh'"
    if ($LASTEXITCODE -eq 1) { $Action = 'deploy' }
    elseif ($LASTEXITCODE -ne 0) { throw 'SSH connection failed.' }
}
if ($Action -eq 'deploy') {
    $ArtifactDir = Join-Path $PSScriptRoot '.local/deploy'
    New-Item -ItemType Directory -Force -Path $ArtifactDir | Out-Null
    $ArchiveName = 'dreamreel-source-' + [guid]::NewGuid().ToString('N') + '.tar.gz'
    $Archive = Join-Path $ArtifactDir $ArchiveName
    $RemoteArchive = '/tmp/' + $ArchiveName
    try {
        # Include uncommitted source changes. Keep runtime data and secrets on the server.
        & tar -czf $Archive --exclude=.git --exclude=.local --exclude=.runtime `
            --exclude=.agents --exclude=.codex --exclude=.aws --exclude=backups `
            --exclude=node_modules --exclude=.npm-cache --exclude=dist --exclude=uploads --exclude=logfile `
            '--exclude=*.exe' '--exclude=.env*' --exclude=config.yaml -C $PSScriptRoot .
        Assert-NativeExit 'Source packaging'
        & scp -q $Archive "${Target}:$RemoteArchive"
        Assert-NativeExit 'Source upload'
        & ssh -o BatchMode=yes $Target "set -eu; mkdir -p '$RemotePath'; chmod 750 '$RemotePath'; tar --no-same-owner -xzf '$RemoteArchive' -C '$RemotePath'; rm -f '$RemoteArchive'; chmod +x '$RemotePath/start.sh'"
        Assert-NativeExit 'Source extraction'
    }
    finally {
        if (Test-Path -LiteralPath $Archive) { Remove-Item -LiteralPath $Archive }
    }
}
# SSH allocates a terminal so sudo can prompt; no password is saved by this script.
& ssh -t $Target "bash '$RemotePath/start.sh' '$Action'"
Assert-NativeExit 'Remote DreamReel operation'
