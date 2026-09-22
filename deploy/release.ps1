# 배포 한 줄: 현재 체크아웃으로 api 이미지를 빌드해 올리고, 실제 경로를 태우는 스모크가 통과해야 끝난다.
#   .\deploy\release.ps1 prod   # .env  + deploy/compose.yml     → http://localhost:8081
#   .\deploy\release.ps1 dev    # .env.dev + deploy/compose.dev.yml → http://localhost:8082 (devtoken 으로 인증 경로까지)
# 이미지에 git 커밋(GIT_SHA)을 박아 /health 의 version 으로 내려주므로, 다른 트리에서 빌드한 이미지는 스모크가 잡는다.
# 수정된 추적 파일이 있으면 중단한다(커밋 안 된 코드로 배포하면 version 이 거짓말이 된다).
param(
    [Parameter(Mandatory = $true)][ValidateSet('prod', 'dev')][string]$Env
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# .env 파일에서 키 하나를 읽는다. Go 쪽 readDotEnv(backend/internal/config/config.go)와 같게 첫 '=' 에서 한 번만
# 나누고 양끝 따옴표를 벗긴다(base64 비밀의 '=' 패딩이 잘리지 않게).
function Read-DotEnvValue([string]$Path, [string]$Key) {
    foreach ($line in Get-Content $Path) {
        $t = $line.Trim()
        if ($t -eq '' -or $t.StartsWith('#')) { continue }
        $i = $t.IndexOf('=')
        if ($i -lt 1 -or $t.Substring(0, $i).Trim() -ne $Key) { continue }
        $v = $t.Substring($i + 1).Trim()
        if ($v.Length -ge 2 -and (($v[0] -eq "'" -and $v[-1] -eq "'") -or ($v[0] -eq '"' -and $v[-1] -eq '"'))) {
            $v = $v.Substring(1, $v.Length - 2)
        }
        return $v
    }
    throw "$Path 에 $Key 가 없습니다"
}

# untracked 도 막는다 — Dockerfile 이 `COPY . .` 라 추적 안 된 .go 파일도 바이너리에 들어가는데 version 은 HEAD 를 말하게 된다.
$dirty = git status --porcelain
if ($dirty) {
    Write-Host "중단: 커밋되지 않은 변경(추적 안 된 파일 포함)이 있습니다. 커밋하거나 되돌린 뒤 배포하세요." -ForegroundColor Red
    $dirty | ForEach-Object { Write-Host "  $_" }
    exit 1
}
$env:GIT_SHA = (git rev-parse --short HEAD).Trim()
$branch = (git rev-parse --abbrev-ref HEAD).Trim()

if ($Env -eq 'prod') {
    $envFile = '.env'; $compose = 'deploy/compose.yml'; $url = 'http://localhost:8081'
} else {
    $envFile = '.env.dev'; $compose = 'deploy/compose.dev.yml'; $url = 'http://localhost:8082'
}
if (-not (Test-Path $envFile)) { Write-Host "중단: $envFile 없음" -ForegroundColor Red; exit 1 }

Write-Host "== $Env 배포: $branch @ $env:GIT_SHA → $url"
docker compose --env-file $envFile -f $compose up -d --build api
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# 기동 대기: /health 가 200 이 될 때까지(최대 60초). OTP 는 이미 떠 있다는 전제(api 만 재생성).
$ok = $false
for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Seconds 2
    try {
        $r = Invoke-WebRequest -Uri "$url/health" -UseBasicParsing -TimeoutSec 5
        if ($r.StatusCode -eq 200) { $ok = $true; break }
    } catch {}
}
if (-not $ok) {
    Write-Host "중단: $url/health 가 60초 안에 200 이 되지 않았습니다" -ForegroundColor Red
    docker compose --env-file $envFile -f $compose logs --tail 30 api
    exit 1
}

# 개발 스택은 devtoken 을 받아 인증 경로·경로 탐색까지 스모크한다(개발 DB 에 smoke 사용자 1명 생김).
$token = ''
if ($Env -eq 'dev') {
    Push-Location backend
    # devtoken 은 config.Load() 로 레포 루트 .env(운영)를 읽으므로 개발 DB 와 개발 JWT_SECRET 을 환경변수로 덮어쓴다
    # (환경변수가 .env 보다 우선). 운영 비밀로 서명하면 개발 api 가 전부 401 이다. 호출한 셸의 값은 되돌린다.
    $prevDb = $env:DATABASE_URL
    $prevJwt = $env:JWT_SECRET
    $env:DATABASE_URL = 'postgres://seoul:seoul@localhost:5433/seoul_route?sslmode=disable'
    $env:JWT_SECRET = Read-DotEnvValue -Path (Join-Path $root $envFile) -Key 'JWT_SECRET' # cwd 가 backend 라 절대경로
    $token = (go run ./cmd/devtoken smoke).Trim()
    $env:DATABASE_URL = $prevDb
    $env:JWT_SECRET = $prevJwt
    Pop-Location
}
& bash deploy/smoke.sh $url $token
$smoke = $LASTEXITCODE
if ($smoke -ne 0) { Write-Host "배포는 됐지만 스모크 실패 — 위 FAIL 줄을 보고 고치거나 이전 커밋으로 다시 배포하세요." -ForegroundColor Red }
exit $smoke
