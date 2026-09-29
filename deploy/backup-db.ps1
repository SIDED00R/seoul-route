# 운영 Postgres 백업: 컨테이너 안에서 pg_dump -Fc 로 뜨고 목록 검사(pg_restore -l)를 통과하면 호스트 백업 폴더로
# 복사한 뒤 최근 $Keep 개만 남긴다. 기본 폴더는 Docker 데이터(D:)와 다른 디스크(C:)다.
#   .\deploy\backup-db.ps1             # 한 번 뜨기
#   .\deploy\backup-db.ps1 -Register   # 작업 스케줄러 등록(로그인 5분 뒤 + 매일 21:00)
# 복구·복구 연습: docs/db-backup.md
param(
    [string]$Dir = "$env:USERPROFILE\seoul-route-db-backup",
    [int]$Keep = 14,
    [switch]$Register
)
# 네이티브 명령은 줄마다 $LASTEXITCODE 로 본다. Stop 이면 호출자가 오류 출력을 리다이렉트할 때(2>&1·2>$null) docker 의
# 오류 출력이 예외가 되어 임시 파일 정리 전에 멈춘다.
$ErrorActionPreference = 'Continue'
$container = 'deploy-postgres-1'

if ($Register) {
    $arg = "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$PSCommandPath`" -Dir `"$Dir`" -Keep $Keep"
    $action = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument $arg
    $logon = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
    $logon.Delay = 'PT5M' # Docker Desktop 이 뜰 시간
    $daily = New-ScheduledTaskTrigger -Daily -At '21:00'
    $settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -ExecutionTimeLimit (New-TimeSpan -Minutes 10)
    Register-ScheduledTask -TaskName 'seoul-route DB 백업' -Action $action -Trigger $logon, $daily `
        -Settings $settings -Force -ErrorAction Stop | Out-Null
    Write-Host "등록: 작업 스케줄러 'seoul-route DB 백업' (로그인 5분 뒤, 매일 21:00) → $Dir"
    exit 0
}

New-Item -ItemType Directory -Force -Path $Dir | Out-Null
$log = Join-Path $Dir 'backup.log'
function Write-Log([string]$msg) {
    $line = "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') $msg"
    Add-Content -Path $log -Value $line -Encoding UTF8
    Write-Host $line
}

$state = docker inspect -f '{{.State.Health.Status}}' $container 2>$null
if ($LASTEXITCODE -ne 0 -or $state -ne 'healthy') { Write-Log "건너뜀: $container 가 healthy 가 아님($state)"; exit 1 }

$name = "seoul_route-$(Get-Date -Format 'yyyyMMdd-HHmmss').dump"
$tmp = "/tmp/$name"
docker exec $container pg_dump -U seoul -Fc -f $tmp seoul_route
if ($LASTEXITCODE -ne 0) { docker exec $container rm -f $tmp | Out-Null; Write-Log '실패: pg_dump'; exit 1 }
# 목차(TOC)를 읽지 못하는 덤프를 거른다. 데이터 부분이 잘린 덤프는 이 검사를 통과한다.
docker exec $container pg_restore -l $tmp | Out-Null
if ($LASTEXITCODE -ne 0) { docker exec $container rm -f $tmp | Out-Null; Write-Log '실패: 덤프 목록 검사'; exit 1 }
$dst = Join-Path $Dir $name
docker cp "${container}:$tmp" $dst | Out-Null
$cpExit = $LASTEXITCODE
docker exec $container rm -f $tmp | Out-Null
if ($cpExit -ne 0) { Write-Log '실패: docker cp'; exit 1 }
Write-Log "완료: $name ($([math]::Round((Get-Item $dst).Length / 1KB)) KB)"

Get-ChildItem $Dir -Filter 'seoul_route-*.dump' | Sort-Object Name -Descending | Select-Object -Skip $Keep |
    ForEach-Object { Remove-Item $_.FullName; Write-Log "삭제(보관 $Keep 개 초과): $($_.Name)" }
exit 0
