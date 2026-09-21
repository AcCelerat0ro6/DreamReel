$ErrorActionPreference = 'Continue'
$base = "http://localhost:8080"
$suffix = Get-Random -Maximum 999999
$pwd = "Test@12345"
$accA = "userA_$suffix"
$accB = "userB_$suffix"

function Show-Err($e) {
    if ($e.Exception.Response) {
        Write-Host "STATUS: $([int]$e.Exception.Response.StatusCode)"
    } else {
        Write-Host "ERROR: $($e.Exception.Message)"
    }
    if ($e.ErrorDetails) { Write-Host "DETAIL: $($e.ErrorDetails.Message)" }
}

function Register-Login($acc) {
    $regBody = @{ account = $acc; password = $pwd; nickname = $acc } | ConvertTo-Json
    try {
        $null = Invoke-WebRequest -Uri "$base/api/users" -Method POST -Body $regBody -ContentType "application/json" -UseBasicParsing -TimeoutSec 5
    } catch { }
    $loginBody = @{ account = $acc; password = $pwd } | ConvertTo-Json
    $r = Invoke-WebRequest -Uri "$base/api/sessions" -Method POST -Body $loginBody -ContentType "application/json" -UseBasicParsing -TimeoutSec 5
    return ($r.Content | ConvertFrom-Json).access_token
}

Write-Host "===== SETUP: register A($accA) and B($accB) ====="
$tokenA = Register-Login $accA
$tokenB = Register-Login $accB
Write-Host "TOKEN A len=$($tokenA.Length)  TOKEN B len=$($tokenB.Length)"

Write-Host "`n===== STEP 1: A creates video V1 ====="
$vbody = @{
    title = "Video To Delete"
    description = "test delete/get chain"
    media_url = "https://cdn.example.com/v/del.mp4"
    cover_url = "https://cdn.example.com/c/del.jpg"
    model_name = "sora-v1"
    ai_style_tag = "test"
} | ConvertTo-Json
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos" -Method POST -Body $vbody -ContentType "application/json" -Headers @{ Authorization = "Bearer $tokenA"; "Idempotency-Key" = "del-$suffix" } -UseBasicParsing -TimeoutSec 8
    Write-Host "STATUS: $($r.StatusCode)"
    $v1 = ($r.Content | ConvertFrom-Json).id
    Write-Host "V1_ID: $v1"
} catch { Show-Err $_; exit 1 }

Write-Host "`n===== STEP 2: A creates video V2 (kept for later) ====="
$vbody2 = @{
    title = "Video Keep"
    description = "second"
    media_url = "https://cdn.example.com/v/keep.mp4"
    cover_url = "https://cdn.example.com/c/keep.jpg"
} | ConvertTo-Json
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos" -Method POST -Body $vbody2 -ContentType "application/json" -Headers @{ Authorization = "Bearer $tokenA" } -UseBasicParsing -TimeoutSec 8
    $v2 = ($r.Content | ConvertFrom-Json).id
    Write-Host "STATUS: $($r.StatusCode)  V2_ID: $v2"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 3: GET V1 without auth (public endpoint, expect 200) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/$v1" -Method GET -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
    Write-Host "BODY: $($r.Content)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 4: GET V1 with tokenB (expect 200, public) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/$v1" -Method GET -Headers @{ Authorization = "Bearer $tokenB" } -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 5: GET nonexistent video (id=99999999, expect 404) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/99999999" -Method GET -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 6: GET invalid id 'abc' (expect 400) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/abc" -Method GET -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 7: GET id=0 (expect 400) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/0" -Method GET -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 8: GET id=-1 (Gin route may reject, expect 400 or 404) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/-1" -Method GET -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 9: DELETE V1 with tokenB (not author, expect 403) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/$v1" -Method DELETE -Headers @{ Authorization = "Bearer $tokenB" } -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
    Write-Host "BODY: $($r.Content)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 10: DELETE V1 without token (expect 401) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/$v1" -Method DELETE -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 11: DELETE V1 with tokenA (author, expect 204) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/$v1" -Method DELETE -Headers @{ Authorization = "Bearer $tokenA" } -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 12: GET V1 after delete (expect 404, soft-deleted filtered out) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/$v1" -Method GET -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
    Write-Host "BODY: $($r.Content)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 13: DELETE V1 again with tokenA (idempotent replay) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/$v1" -Method DELETE -Headers @{ Authorization = "Bearer $tokenA" } -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 14: DELETE V2 with tokenB (not author, expect 403) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/$v2" -Method DELETE -Headers @{ Authorization = "Bearer $tokenB" } -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 15: GET V2 (still exists, expect 200) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/$v2" -Method GET -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
    $obj = $r.Content | ConvertFrom-Json
    Write-Host "id=$($obj.id) status=$($obj.status) title=$($obj.title)"
} catch { Show-Err $_ }

Write-Host "`n===== STEP 16: DELETE V1 with tokenA after already deleted (permission still checked?) ====="
try {
    $r = Invoke-WebRequest -Uri "$base/api/videos/$v1" -Method DELETE -Headers @{ Authorization = "Bearer $tokenA" } -UseBasicParsing -TimeoutSec 5
    Write-Host "STATUS: $($r.StatusCode)"
} catch { Show-Err $_ }
