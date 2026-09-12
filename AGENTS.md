# seoul-route

서울시 한정 멀티모달 길찾기 서비스. 대중교통·따릉이·걷기를 한 경로 안에서 섞어 최저시간순으로 추천하고, 사용자별 실측 이동속도와 횡단보도 대기시간을 예상시간에 반영한다.

## 구조

```
otp/       OpenTripPlanner 2.10.0 설정·데이터 스크립트 (그래프 빌드는 로컬, 서빙은 VM)
backend/   Go API (chi + pgx/sqlc). OTP 오케스트레이션, 궤적 수집, 따릉이 GBFS 어댑터, 신호 잔여시간 조회
gtfs/      GTFS 생성기 (서울 버스 API + TAGO 지하철 시간표 → GTFS zip)
app/       Flutter 앱
deploy/    Compose·Caddy·GCE 배포
docs/      스파이크 보고서·설계 문서
```

## 실행

- OTP 2.10.0 은 **Java 25** 가 필요하다(class file 69, Java 21 은 UnsupportedClassVersionError).
- 입력 데이터 준비(jar 은 `otp/`, 나머지는 `otp/data/` — 전부 git 무시):
  - OTP 실행파일: GitHub `opentripplanner/OpenTripPlanner` 릴리스 v2.10.0 의 `otp-shaded-2.10.0.jar` → `otp/`
  - OSM: Geofabrik `asia/south-korea-latest.osm.pbf` → `otp/data/` → `python otp/extract_seoul.py` 로 `otp/data/seoul.osm.pbf`
  - GTFS 파일럿: 국가교통DB(ktdb.go.kr) 로그인 → 정보공개 > 자료신청 > 교통분석자료 신청 > 교통망 GIS DB > 대중교통 > 대중교통 GTFS(2025-03) 신청·다운로드 → zip 을 풀어 `otp/data/202503_GTFS_DataSet/` 에 배치 → `python otp/filter_gtfs_seoul.py` 로 `otp/data/gtfs-ktdb.zip` 생성(서울 bbox 필터 + route_type 표준 변환)
  - 운영 GTFS: `cd gtfs && go run ./cmd/gtfsgen fetch && go run ./cmd/gtfsgen build` → `gtfs/out/seoul-gtfs.zip` 을 `otp/data/seoul-gtfs.zip` 으로 복사. build-config 의 transitFeeds 는 이 파일을 가리킨다. 상세 `docs/gtfs-generator.md`
  - GTFS zip 이 없으면 빌드는 **실패하지 않고** 종료코드 0 으로 `|Stops|=0` 그래프가 나온다(실측 2026-09-12). 빌드 로그의 `Transit built. |Stops|=` 를 확인한다.
- OTP 그래프 빌드: `cd otp && java -Xmx8G -jar otp-shaded-2.10.0.jar --build --save .` (운영 GTFS 최종본 기준 peak RSS 4.1GB 실측, 로컬 PC 에서만)
- OTP 서빙: `cd otp && java -Xmx4G -jar otp-shaded-2.10.0.jar --load .` → GraphQL `http://localhost:8080/otp/gtfs/v1` (운영 GTFS 그래프 서빙 RSS 2.2GB, 질의 3건 후 실측)
- GBFS fixture 서버(스파이크용): `python -m http.server 8090 -d otp/fixtures/gbfs`
- PostgreSQL 17(로컬, WSL 없이): EDB 포터블 바이너리 `C:\Users\SAMSUNG\tools\pg17\pgsql`, 데이터 `..\pg17\data`, 포트 5432, 역할 `seoul/seoul`, DB `seoul_route`·`seoul_route_test`. 기동은 PowerShell `Start-Process postgres.exe -ArgumentList '-D',<data>,'-p','5432' -WindowStyle Hidden` (셸 자식으로 띄우면 셸 종료 시 같이 죽는다 — 실측). PostGIS 는 없다 → Phase 4 부터 Compose 의 `postgis/postgis:17-3.5` 를 쓴다.
- 백엔드: `cd backend && go run ./cmd/api` → `http://localhost:8081`. 설정은 `.env`(DATABASE_URL·OTP_URL·JWT_SECRET 필수, GOOGLE_OAUTH_CLIENT_ID 없으면 `/auth/google` 503). 기동 시 `internal/db/migrations/*.sql` 을 자동 적용한다.
- 전체 스택(Compose, WSL 필요): `docker compose -f deploy/compose.yml up -d` (postgis·otp·api)

## 검증

- Go: `cd gtfs && go vet ./... && go test ./...` / `cd backend && go vet ./... && TEST_DATABASE_URL=postgres://seoul:seoul@localhost:5432/seoul_route_test?sslmode=disable go test ./...` (DB 없으면 httpapi 테스트는 skip 된다 — 통과가 아니다)
- API 실행 검증: 서버 기동 후 `curl localhost:8081/health`(db·otp 둘 다 ok 인지 본문 확인), 미인증 `/users/me` 401, `/auth/google` 미설정 503
- Flutter: `cd app && flutter analyze && flutter test`
- 기능 완료 판정은 테스트 통과가 아니라 실제 실행이다: OTP에 curl로 plan 요청을 보내 응답 본문을 읽고, 앱은 실기기에서 흐름을 태운다.

## 데이터·키

- 인증키는 `.env`(gitignore)에만 둔다. `.env.example`에 변수명이 있다. 채팅·로그·URL 출력에 키를 남기지 않는다.
- 국가교통DB GTFS 파일럿(2025-03 평일 1일)은 엔진 스파이크용이다. 운영 시간표는 `gtfs/` 생성기가 만든다.
- T-data 신호 잔여시간 API는 개발자 한도 하루 1,000건, 잔여시간 단위 1/10초, 좌표 없음.
- 열린데이터광장 `bikeList`는 1콜 최대 1,000행. `list_total_count`는 전체가 아니라 요청 범위 건수를 돌려주므로(실측) 짧은 페이지가 나올 때까지 넘긴다. 2026-09-12 실측 2,734곳 = 3콜.
- 공공데이터포털 키는 계정당 1개이며 API마다 활용신청이 필요하다. 서울 버스 = `ws.bus.go.kr/api/rest/busRouteInfo/*`(노선별 정류장에 좌표·구간거리·정류장별 첫막차 포함). TAGO 지하철 = `apis.data.go.kr/1613000/SubwayInfo/Get*`(2022-09 개편, 구 `SubwayInfoService/get*` 경로는 오류). 역 검색 응답에 좌표가 없다.

## 규칙

- Issue → 브랜치(`feat/…` `fix/…`) → PR(본문 `Closes #n`) → 리뷰 → Merge. main 직접 커밋 금지.
- 커밋 메시지에 AI 트레일러를 넣지 않는다.
- 파일은 단일 책임. 기능 추가는 새 파일로.
- 상수(속도 사전값·횡단보도 대기·필터 임계)는 출처·측정일·재보정 규칙을 주석으로 붙인다.
- 버전 고정: OTP 2.10.0, Go 1.27, PostgreSQL 17, Flutter stable(설치 시 확정).
