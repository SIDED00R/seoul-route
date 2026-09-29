# 운영 DB 백업·복구 (이슈 #149)

운영 Postgres(`deploy-postgres-1`, 볼륨 `deploy_pgdata`)에는 걷기·자전거 속도 학습, 즐겨찾기, 최근 경로, 안내 궤적(30일)이 있다.
PC 를 끄면 Docker VM 이 통째로 멈춰 Postgres 는 다음 기동 때 "비정상 종료 후 자동 복구"(WAL 재생)로 시작한다. 전원이 나가도
데이터가 보존되도록 만든 동작이라 따로 끌 필요는 없다. 백업은 Docker 디스크 손상·재설치·볼륨 삭제처럼 사본이 없어지는 경우를 대비한다.

## 백업

- `deploy/backup-db.ps1`: 컨테이너 안에서 `pg_dump -Fc` → `pg_restore -l` 로 목록 검사 → `%USERPROFILE%\seoul-route-db-backup\seoul_route-<시각>.dump`
  로 복사 → 최근 14개만 남긴다. 기록은 같은 폴더 `backup.log`. Docker 데이터(D:)와 다른 디스크(C:)다.
- 작업 스케줄러 `seoul-route DB 백업`: 로그인 5분 뒤 + 매일 21:00(`.\deploy\backup-db.ps1 -Register` 로 등록). Postgres 가 healthy 가
  아니면 건너뛰고 로그에 남긴다.
- `deploy/release.ps1 prod` 는 배포 직전에 한 번 뜬다(마이그레이션은 api 기동 때 자동 적용되고 되돌리는 기능이 없다).
  Postgres 가 아직 없으면 `-SkipBackup`.

## 복구

복원은 DB 를 지우므로 먼저 `.\deploy\backup-db.ps1` 로 지금 상태를 한 번 떠 둔다(보관 14개 규칙에 복원할 파일이 밀려
지워지지 않는지도 본다). PowerShell 은 네이티브 명령이 실패해도 다음 줄로 가므로 줄마다 `$LASTEXITCODE` 를 본다.

```powershell
docker stop deploy-api-1                                   # 복구 중 쓰기를 막는다. 복원이 성공했을 때만 다시 켠다
if ($LASTEXITCODE -eq 0) { docker cp C:\Users\SAMSUNG\seoul-route-db-backup\<파일>.dump deploy-postgres-1:/tmp/restore.dump }
if ($LASTEXITCODE -eq 0) { docker exec deploy-postgres-1 pg_restore -l /tmp/restore.dump | Out-Null }
# 덤프에 없는 객체(백업 뒤 마이그레이션이 만든 테이블 등)가 남지 않도록 DB 를 지우고 빈 DB 에 복원한다. FORCE 는 남은 연결을 끊는다.
if ($LASTEXITCODE -eq 0) { docker exec deploy-postgres-1 psql -U seoul -d postgres -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS seoul_route WITH (FORCE)" -c "CREATE DATABASE seoul_route" }
# 한 트랜잭션으로 복원한다. 중간에 실패하면 전부 취소되어 빈 DB 로 남는다.
if ($LASTEXITCODE -eq 0) { docker exec deploy-postgres-1 pg_restore -U seoul -d seoul_route --no-owner --single-transaction /tmp/restore.dump }
if ($LASTEXITCODE -eq 0) { docker exec deploy-postgres-1 rm -f /tmp/restore.dump; docker start deploy-api-1 } else { Write-Host "복원 실패(exit $LASTEXITCODE): api 를 켜지 않았다" -ForegroundColor Red }
```

볼륨을 새로 만든 경우(빈 Postgres)도 같다. DB 를 지우고 새로 만든 뒤 복원하므로 덤프 뒤에 생긴 객체는 남지 않는다.
2026-09-29 개발 Postgres 에서 위 블록을 실행해 정상 덤프·잘린 덤프·경로 오타·덤프 아닌 파일 네 경우를 확인했다(잘린
덤프는 DROP 뒤 롤백되어 빈 DB 로 남고 api 는 켜지 않는다).

배포를 되돌리려는 복원이면 마지막 줄의 `docker start deploy-api-1` 을 빼고, 덤프를 뜬 시점의 커밋을 체크아웃해
`.\deploy\release.ps1 prod` 로 api 를 다시 배포한다 — 지금 이미지의 api 를 켜면 기동 때 같은 마이그레이션을 다시
적용한다.

## 복구 연습

개발 Postgres 에 임시 DB 로 복원해 운영과 테이블별 행 수를 대조한 뒤 지운다(2026-09-29: users 1·trips 33·traces 5,636·
speed_profiles 2·favorite_places 1·recent_routes 20·schema_migrations 6, 운영과 전부 같음).

```powershell
docker cp C:\Users\SAMSUNG\seoul-route-db-backup\<파일>.dump seoul-route-dev-postgres-1:/tmp/drill.dump
docker exec seoul-route-dev-postgres-1 psql -U seoul -d seoul_route -c "CREATE DATABASE restore_drill"
docker exec seoul-route-dev-postgres-1 pg_restore -U seoul -d restore_drill --no-owner --single-transaction /tmp/drill.dump
# 운영(deploy-postgres-1, seoul_route)과 복원본(seoul-route-dev-postgres-1, restore_drill)에서 테이블별 count(*) 비교
docker exec seoul-route-dev-postgres-1 psql -U seoul -d seoul_route -c "DROP DATABASE restore_drill"
docker exec seoul-route-dev-postgres-1 rm -f /tmp/drill.dump
```
