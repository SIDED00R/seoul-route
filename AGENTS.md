# seoul-route

서울시 한정 멀티모달 길찾기 서비스. 대중교통·따릉이·걷기를 한 경로 안에서 섞어 최저시간순으로 추천하고, 사용자별 실측 이동속도와 횡단보도 대기시간을 예상시간에 반영한다.

## 구조

```
otp/       OpenTripPlanner 2.10.0 설정·데이터 스크립트 (그래프 빌드·서빙 모두 로컬 PC)
backend/   Go API (chi + pgx). OTP 오케스트레이션, 따릉이 GBFS 어댑터, 카카오 검색·VWorld 타일 프록시, 궤적 수집
gtfs/      GTFS 생성기 (서울 버스 API + TAGO 지하철 시간표 → GTFS zip)
app/       Flutter 앱(Android 전용). lib/{api,models,screens,settings,util,widgets,guide(안내 구간 추적·궤적 업로더)}
deploy/    로컬 Docker Compose(postgis·otp·api). 클라우드 배포는 하지 않는다(2026-09-12 결정)
docs/      스파이크 보고서·설계 문서
```

## 실행

- OTP 2.10.0 은 **Java 25** 가 필요하다(class file 69, Java 21 은 UnsupportedClassVersionError).
- 입력 데이터 준비(jar 은 `otp/`, 나머지는 `otp/data/` — 전부 git 무시):
  - OTP 실행파일: GitHub `opentripplanner/OpenTripPlanner` 릴리스 v2.10.0 의 `otp-shaded-2.10.0.jar` → `otp/`
  - OSM: Geofabrik `asia/south-korea-latest.osm.pbf` → `otp/data/` → `python otp/extract_seoul.py` 로 `otp/data/seoul.osm.pbf` → `python otp/extract_entrances.py` 로 `otp/data/subway-entrances.csv`(지하철 출입구 2,457개, 공사 중 제외. GTFS 생성기가 읽는다 — 없으면 승강장 좌표 출입구로 폴백)
  - GTFS 파일럿: 국가교통DB(ktdb.go.kr) 로그인 → 정보공개 > 자료신청 > 교통분석자료 신청 > 교통망 GIS DB > 대중교통 > 대중교통 GTFS(2025-03) 신청·다운로드 → zip 을 풀어 `otp/data/202503_GTFS_DataSet/` 에 배치 → `python otp/filter_gtfs_seoul.py` 로 `otp/data/gtfs-ktdb.zip` 생성(서울 bbox 필터 + route_type 표준 변환)
  - 신호 횡단보도: `python otp/extract_crossings.py` → `otp/data/crossings.csv`(OSM `highway=crossing` 중 신호 태그 또는 trunk/primary/secondary/tertiary 위 무태그 노드, 10,477곳). 백엔드가 기동 시 읽어 도보·따릉이 leg 마다 기대 대기 38초/곳을 더한다(`CROSSINGS_CSV`, Compose 는 `/data/crossings.csv` 마운트, 없으면 미반영 경고). 상세 `docs/crossing-wait.md`.
  - 지하철 시간표: `python otp/fetch_metro_timetable.py` → `otp/data/seoul-metro-timetable.csv`(공공데이터포털 서울교통공사 열차운행시각표, 1~9호선 요일별·열차코드, 32MB). 생성기가 있으면 1~9호선 trip 을 이걸로 만들고(service WEEKDAY/SAT/SUN) 없으면 파일럿 trip 을 쓴다.
  - 코레일·민자 노선 시간표: `python otp/fetch_kric_timetable.py` → `otp/data/kric-stations.csv`·`kric-timetable.csv`(레일포털 data.kric.go.kr Open API, `.env` `KRIC_API_KEY` 필요 — 회원가입 후 인증키 발급·API 활용신청: 도시철도 전체노선정보·역사별 정보·역사별 운행시각표. 12개 노선(경의중앙·수인분당·경춘·경강·공항철도·신분당·의정부·신림·우이신설·김포골드·인천1·2호선) 역별·요일별, 약 300역×3회 호출 20분). 생성기가 있으면 그 노선 파일럿 trip 을 버리고 시각표 trip 을 만든다(토요일 시각표가 없는 노선은 휴일 시각표를 토·일 SATSUN 에). 서해선(KRIC 에 소사~원시 12역만)·GTX-A(없음)는 파일럿 그대로(ALL, 평일 1일 표본이라 구멍 있음). 상세 `docs/gtfs-generator.md`.
  - 운영 GTFS: `cd gtfs && go run ./cmd/gtfsgen fetch && go run ./cmd/gtfsgen build` → `gtfs/out/seoul-gtfs.zip` 을 `otp/data/seoul-gtfs.zip` 으로 복사. build-config 의 transitFeeds 는 이 파일을 가리킨다. 지하철 환승·출입구는 `pathways.txt` 로 고정한다: 승강장 간 통로(다승강장 104역) + OSM 실제 출입구(가장 가까운 승강장 350m 안, 454역, 진입 60초·이탈 30초 + 거리÷1.0 m/s, 500m 안 승강장에만 연결). OSM 출입구가 없는 다승강장 역은 승강장 좌표 출입구(진입 120·이탈 60초 = 백엔드 `route/station_slack.go` 와 같은 값)로 폴백. 상세 `docs/gtfs-generator.md`
  - GTFS zip 이 없으면 빌드는 **실패하지 않고** 종료코드 0 으로 `|Stops|=0` 그래프가 나온다(실측 2026-09-12). 빌드 로그의 `Transit built. |Stops|=` 를 확인한다.
- OTP 그래프 빌드: `cd otp && java -Xmx8G -jar otp-shaded-2.10.0.jar --build --save .` (운영 GTFS 최종본 기준 peak RSS 4.1GB 실측, 로컬 PC 에서만)
- OTP 서빙: `cd otp && java -Xmx4G -jar otp-shaded-2.10.0.jar --load .` → GraphQL `http://localhost:8080/otp/gtfs/v1` (운영 GTFS 그래프 서빙 RSS 2.2GB, 질의 3건 후 실측)
- GBFS fixture 서버(스파이크용): `python -m http.server 8090 -d otp/fixtures/gbfs`
- PostgreSQL 17(로컬, WSL 없이): EDB 포터블 바이너리 `C:\Users\SAMSUNG\tools\pg17\pgsql`, 데이터 `..\pg17\data`, 포트 5432, 역할 `seoul/seoul`, DB `seoul_route`·`seoul_route_test`. 기동은 PowerShell `Start-Process postgres.exe -ArgumentList '-D',<data>,'-p','5432' -WindowStyle Hidden` (셸 자식으로 띄우면 셸 종료 시 같이 죽는다 — 실측). PostGIS 는 없다 → Phase 4 부터 Compose 의 `postgis/postgis:17-3.5` 를 쓴다.
- 백엔드: `cd backend && go run ./cmd/api` → `http://localhost:8081`. 설정은 `.env`(DATABASE_URL·OTP_URL·JWT_SECRET 필수, GOOGLE_OAUTH_CLIENT_ID 없으면 `/auth/google` 503, SEOUL_OPENAPI_KEY 없으면 `/gbfs/*` 503). 기동 시 `internal/db/migrations/*.sql` 을 자동 적용한다.
  - 엔드포인트: `GET /health` · `GET /auth/config`(앱이 쓸 Google 웹 클라이언트 ID, 미설정이면 빈 값 — 공개 식별자라 무인증) · `POST /auth/google`(Google ID 토큰 → 서버 JWT) · `GET /gbfs/{gbfs,system_information,station_information,station_status}.json`(따릉이 60초 폴링, OTP 가 읽는다) · 인증 필요: `GET/DELETE /users/me`, `POST /routes/plan`(origin·destination·via[] 는 lat·lon·name, segment_modes[]·depart. name 이 "…역" 이고 1km 안에 같은 이름의 GTFS 부모역이 있으면 좌표 대신 역 ID 로 OTP 에 요청한다 — 역사 좌표가 선로 반대편 도로에 붙는 문제 회피, 이슈 #12), 응답 itinerary 의 `depart_in_sec`(지금 출발 시 출발까지 대기)·`duration_sec`(출발부터 도착. 역 ID 로 앵커링된 출발지는 출입구→승강장 진입 2분, 도착지는 이탈 1분이 더해지고 `start`/`end` 도 그만큼 벌어진다 — `route/station_slack.go`)·`realtime`/`realtime_delta_sec`(첫 탑승을 버스 도착정보·지하철 실시간 도착으로 보정했을 때, DATA_GO_KR_KEY·SEOUL_SUBWAY_REALTIME_KEY 필요, 노선·역별 30초 캐시, 미래 출발은 미적용). 순위와 앱 표시는 `depart_in_sec + duration_sec` 기준. 도보·따릉이 leg 의 `crossings`/`crossing_wait_sec`(지나는 신호 횡단보도 수·기대 대기, `duration_sec` 에 포함)와 여정 `crossing_wait_sec`·`replanned`(대기로 탑승을 놓쳐 그 정류장에서 재탐색한 여정. via 요청은 재탐색하지 않는다) — `route/crossing_hook.go`. 대중교통 leg 의 `headway_sec`(버스, `GTFS_ZIP`=생성 GTFS 의 frequencies, 기본 `otp/data/seoul-gtfs.zip`)·`prev_departures`/`next_departures`(지하철, OTP previousLegs/nextLegs 중 ±3시간·소요시간 0.5~2배 — 순환선 반대 방향 열차 제외)·`realtime_arrivals_sec`(첫 탑승 정류장 실시간 다음 차)는 앱 상세의 "앞뒤 차·배차" 한 줄에 쓴다. `GET /places/search?q=`(카카오 로컬 키워드, 서울 bbox, KAKAO_REST_API_KEY 없으면 503), `GET /tiles/{z}/{x}/{y}.png`(VWorld WMTS Base 프록시, VWORLD_API_KEY 없으면 503). 카카오·VWorld 키는 서버에만 두고 앱은 이 두 경로로만 쓴다.
  - OTP 는 `otp/router-config.json` 의 GBFS url 로 이 서버(8081/gbfs)를 읽는다. 백엔드를 먼저 띄우고 OTP 를 띄우거나, OTP 가 1분마다 재시도하게 둔다.
  - 안내 궤적·속도 학습(인증 필요, `httpapi/trips_handler.go`·`internal/speed`, 상세 `docs/speed-learning.md`): `POST /trips`(안내 1회 = trip 발급) → `POST /trips/{id}/traces`(samples[] = ts·lat·lon·accuracy_m·mode walk/bicycle/transit·선택 activity walk/bicycle/vehicle/still/unknown(폰 활동 인식 판정 — 이동 활동인데 mode 와 다른 샘플 쌍은 속도에서 빼고 종료 응답 `mismatch` 로 센다), 1~1000개, (trip, ts) 중복은 무시하므로 재전송해도 된다, 서울 밖 좌표·잘못된 mode·서버 시각보다 5분 넘게 미래인 ts 400, 남의 trip 404, 종료된 trip 409) → `POST /trips/{id}/end`(수단별 이동 중 중앙값 속도를 내 `speed_profiles` 에 수축 반영, 두 번 부르면 409 — 동시에 겹쳐도 `ended_at IS NULL` 조건부 갱신으로 한 번만 반영. 앱은 409 를 완료로 본다) · `GET /users/me/speed`(walk·bicycle 각 speed_mps·n_trips·prior_mps). `/routes/plan` 은 이 프로파일을 OTP walk/bicycle speed 로 넣고 응답 `walk_speed`/`bike_speed` 로 돌려준다. 원본 궤적은 30일 뒤 지운다(api 가 1시간마다), 탈퇴 시 trip 과 함께 즉시.
  - 개발 토큰(Google 로그인 전): `cd backend && go run ./cmd/devtoken <이름>` → JWT 출력. 운영 이미지에는 넣지 않는다.
- 전체 스택(Compose, WSL 필요): 레포 루트에서 `docker compose --env-file .env -f deploy/compose.yml up -d` (postgis·otp·api). 루트 `.env` 의 JWT_SECRET·SEOUL_OPENAPI_KEY 가 컨테이너로 들어간다. otp 컨테이너는 `deploy/otp/router-config.json`(GBFS url = `http://api:8081`)을 쓴다 — `otp/router-config.json` 의 routingDefaults 를 바꾸면 같이 맞춘다. 로컬 OTP·API 가 8080·8081 을 잡고 있으면 포트 충돌이므로 먼저 내린다. 2026-09-12 Docker Desktop(WSL2)에서 실기동 검증함. 주의: otp 이미지 entrypoint 가 디렉터리 인자를 붙이므로 command 는 `--load` 만, 그리고 OTP 는 설정 파일 주석 안의 달러-중괄호도 환경변수로 치환하므로 그 표기를 쓰지 않는다.
- 앱(Flutter 3.47.4 stable, Android 전용): SDK `C:\Users\SAMSUNG\tools\flutter`, Android SDK `%LOCALAPPDATA%\Android\Sdk`(cmdline-tools 로 설치, Android Studio 는 있으나 GUI 마법사는 안 돌림), Gradle 용 JDK 21 `C:\Users\SAMSUNG\tools\jdk-21.0.12.1+1`(`flutter config --jdk-dir`). 에뮬레이터 AVD `s23ultra`(갤럭시 S23 울트라 사양 1440×3088·560dpi, API 36) = `%LOCALAPPDATA%\Android\Sdk\emulator\emulator.exe -avd s23ultra`. 실행 `cd app && flutter run`(에뮬레이터/USB 실기기). 앱은 설정 화면에서 서버 주소를 넣고(에뮬레이터 `http://10.0.2.2:8081`, USB 실기기는 `adb reverse tcp:8081 tcp:8081` 뒤 `http://127.0.0.1:8081` — Compose 가 loopback 에만 열려 있어 Wi-Fi 로는 못 붙는다. 집 밖에서는 Tailscale: PC·폰에 같은 계정으로 설치하고 PC 에서 `tailscale serve --bg --http=8081 http://127.0.0.1:8081` 한 뒤 앱 서버 주소를 `http://<PC 호스트명>.<tailnet>.ts.net:8081` 로 — Tailscale IP 로 직접 붙으면 404, 호스트명으로만 통한다. 상태는 `tailscale serve status`) "Google 계정으로 로그인"(`lib/auth/google_login.dart`: `/auth/config` 의 웹 클라이언트 ID → `google_sign_in` 7.x Credential Manager → ID 토큰 → `/auth/google` → JWT 저장)하거나 개발용으로 devtoken JWT 를 붙여 넣는다. 홈의 출발지 줄에는 현재 위치 버튼이 있다(`lib/location/current_location.dart`, geolocator 15초 제한 — 이름이 "현재 위치"라 서버 역 앵커링 대상이 아니고, 주소 자리에는 좌표 대신 정확도만 보여 준다. 실기기 화면 `docs/app/19-current-location.png`). Google 로그인에는 Google Cloud 콘솔에 클라이언트 두 개가 필요하다: **웹**(그 ID 를 `.env` `GOOGLE_OAUTH_CLIENT_ID` 에)과 **Android**(패키지 `kr.seoulroute.seoul_route` + 빌드 서명 SHA-1. 디버그 키는 `keytool -list -v -keystore ~/.android/debug.keystore -alias androiddebugkey -storepass android`). 동의 화면 테스트 사용자에 본인 계정 추가. 평문 HTTP 라 매니페스트에 `usesCleartextTraffic` 을 켜 뒀다(개인용·미배포).

## 검증

- Go: `cd gtfs && go vet ./... && go test ./...` / `cd backend && go vet ./... && TEST_DATABASE_URL=postgres://seoul:seoul@localhost:5432/seoul_route_test?sslmode=disable go test ./...` (DB 없으면 httpapi 테스트는 skip 된다 — 통과가 아니다)
- API 실행 검증: 서버 기동 후 `curl localhost:8081/health`(db·otp 둘 다 ok 인지 본문 확인), 미인증 `/users/me` 401, `/auth/google` 미설정 503, `/gbfs/station_status.json` 에 대여소 2,700여 곳, devtoken 으로 `POST /routes/plan` 서울역→강남 정상 응답(legs 본문 확인)·부산 좌표 400
- Flutter: `cd app && flutter analyze && flutter test`. 실행 검증은 에뮬레이터(s23ultra) 또는 실기기에서 설정→검색→경로 목록→상세 지도→안내 시작→종료까지 실제로 눌러 보고 `flutter run` 콘솔에 예외 0건인지 본다. 스크린샷은 `docs/app/` 에 남긴다. 안내 화면의 위치는 에뮬레이터에서 `adb emu geo fix <lon> <lat>` 를 5초 간격으로 흘려 흉내 낸다(`docs/speed-learning.md` 의 검증 절차).
- 기능 완료 판정은 테스트 통과가 아니라 실제 실행이다: OTP에 curl로 plan 요청을 보내 응답 본문을 읽고, 앱은 실기기에서 흐름을 태운다.
- 경로 정확도 대조: OTP 가 떠 있는 상태에서 `cd backend && go run ./cmd/odcompare -at 08:30`(대표 OD 20쌍 = ODsay 20콜, `-n 5` 로 줄임, `-at` 을 빼면 지금 출발·실시간 보정 포함, `-ref ../docs/eval/<이전>.json` 을 주면 그 파일의 ODsay 값을 재사용해 0콜로 돈다(선택한 OD 가 그 파일에 없으면 호출 없이 종료코드 2) — 설정 전후 비교는 이걸로. `-on 2026-09-14` 처럼 날짜를 주면 그 요일 시간표로 돈다 — 지하철이 요일별이라 평일 비교는 평일 날짜로). 결과 `docs/eval/<시각>.md` 의 Δ 중앙값·top3 일치율을 직전 파일과 비교한다. OTP 설정을 바꿨으면 `docker compose ... up -d --force-recreate otp`(`up -d` 는 바인드된 설정 파일 변경을 재생성 사유로 보지 않아 옛 설정으로 계속 돈다 — 2026-09-13 실측). 시간표·순위를 바꾼 PR 은 전후 실행 결과를 `docs/routing-accuracy.md` 에 적는다. `-ref` 없이 `ODSAY_API_KEY` 도 없으면 종료코드 2.

## 데이터·키

- 인증키는 `.env`(gitignore)에만 둔다. `.env.example`에 변수명이 있다. 채팅·로그·URL 출력에 키를 남기지 않는다.
- 국가교통DB GTFS 파일럿(2025-03 평일 1일)은 엔진 스파이크용이다. 운영 시간표는 `gtfs/` 생성기가 만든다.
- T-data 신호 잔여시간 API는 개발자 한도 하루 1,000건, **같은 API 5분에 1회**(초과 시 429 `V2X_REPEAT_CALL_LIMIT`), 잔여시간 단위 1/10초, 좌표 없음. 한 페이지가 교차로 1곳의 지난 로그라 경로 요청 실시간 반영에는 쓰지 않는다(`docs/spike-report.md` 게이트 b, 2026-09-15).
- 열린데이터광장 `bikeList`는 1콜 최대 1,000행. `list_total_count`는 전체가 아니라 요청 범위 건수를 돌려주므로(실측) 짧은 페이지가 나올 때까지 넘긴다. 2026-09-12 실측 2,734곳 = 3콜.
- ODsay Lab(lab.odsay.com) 키는 개인 무료 하루 30콜. `cmd/odcompare` 만 쓰고 서버는 읽지 않는다. 카카오·네이버는 대중교통 경로 API 가 없다.
- 공공데이터포털 키는 계정당 1개이며 API마다 활용신청이 필요하다. 서울 버스 = `ws.bus.go.kr/api/rest/busRouteInfo/*`(노선별 정류장에 좌표·구간거리·정류장별 첫막차 포함). TAGO 지하철 = `apis.data.go.kr/1613000/SubwayInfo/Get*`(2022-09 개편, 구 `SubwayInfoService/get*` 경로는 오류). 역 검색 응답에 좌표가 없다.

## 규칙

- Issue → 브랜치(`feat/…` `fix/…`) → PR(본문 `Closes #n`) → 리뷰 → Merge. main 직접 커밋 금지.
- 커밋 메시지에 AI 트레일러를 넣지 않는다.
- 파일은 단일 책임. 기능 추가는 새 파일로.
- 상수(속도 사전값·횡단보도 대기·필터 임계)는 출처·측정일·재보정 규칙을 주석으로 붙인다.
- 버전 고정: OTP 2.10.0, Go 1.27, PostgreSQL 17, Flutter stable(설치 시 확정).
