# Phase 2a — Flutter 앱 MVP 실행 검증 (2026-09-12)

> 2026-09-16 재검증: Galaxy S23 Ultra에 최신 디버그 APK를 재설치해 홈 화면과 Android 오류 로그 없음을 확인했다. JWT 저장은 Android Keystore 기반으로 전환했으며 기존 평문 키의 자동 이전·삭제도 실기기에서 확인했다. 최신 화면은 [`docs/app/11-current-home.png`](app/11-current-home.png)다.

환경: Flutter 3.47.4 stable, Android SDK 36, 에뮬레이터 `s23ultra`(갤럭시 S23 울트라 사양 1440×3088·560dpi, API 36),
백엔드는 로컬 Docker Compose(postgis·otp·api), 앱 서버 주소 `http://10.0.2.2:8081`, devtoken JWT.

## 흐름 (adb 로 실제 탭·입력, 스크린샷 `docs/app/`)

| 단계 | 결과 | 스크린샷 |
|---|---|---|
| 첫 실행 | 설정 안내 카드, 탐색 버튼 비활성 | 01-home.png |
| 설정 → 연결 확인 | `서버 OK (db ok, otp ok)`, `/users/me` 인증 통과 | — |
| 출발지 검색 `seoul station` | 카카오 결과 10건(서울역·공항철도·1호선…) | 02-search.png |
| 서울역 → 강남역 2호선, 전체 | 후보 4개: 버스 421 55분 / 262+402 56분 / 262+741 57분 / 따릉이 74분. API 200 7.9초 | 03-results.png |
| 후보 1 상세 | VWorld 타일(서버 프록시) 23장 전부 200, 버스 실선·도보 점선·출발/도착 마커 | 04-detail-bus.png |
| 경유 여의도역 5호선 추가, 구간1 따릉이·구간2 대중교통 | 구간 고정 UI | 05-via-modes.png |
| 탐색 | 후보 1개: 도보 9분 → 따릉이 30분(826 서울역 서부교차로2 → 4589 KRX) → 도보 → 9호선/버스 360 → 강남. API 200 | — |
| 상세 | 대여(자전거)·반납(P) 마커, 경유 깃발, 구간 경계 이름을 좌표로 치환(Origin → 서울역, Destination → 여의도역 5호선) | 06-detail-bike-transit.png |

`adb logcat -s flutter AndroidRuntime:E` 예외 0건. `flutter analyze` 0건.
`flutter test` 8건 통과(모델·폴리라인·위젯 4건 + Codex 지적 회귀 4건).

## 추가 (2026-09-13, 이슈 #14·#17)

| 항목 | 내용 | 스크린샷 |
|---|---|---|
| 결과 목록 배지 | "N분 후 출발"(출발 대기), "실시간 ±M분"(첫 탑승 실시간 보정에 따른 도착 이동량, 도착이 그대로면 "실시간"). 총 소요 = 출발 대기 + 소요 | 07-results-realtime.png |
| 상세 앞뒤 차·배차 | 대중교통 leg 아래 한 줄: 실시간 다음 차(첫 탑승) · 앞차/다음 열차 시각(지하철, OTP) · 배차 약 N분(버스, GTFS) | 08-detail-schedule-retina.png |
| 타일 선명도 | `TileLayer(retinaMode: true)` — 한 단계 높은 줌 타일을 절반 크기로 그려 560dpi 에서 또렷하다(지명은 작아짐) | 08-detail-schedule-retina.png |

에뮬레이터 한국어 자판 켠 뒤 adb 영문 입력은 언어 전환 키(keyevent 204)로 영문 자판으로 바꿔야 들어간다.

## 추가 (2026-09-13, 이슈 #32 안내·속도 학습)

상세 화면 "안내 시작" → 안내 화면(현재 구간 강조·내 위치·이전/다음 구간·종료) → 종료 대화상자(이번 안내 속도·내 속도).
설정 "연결 확인" 이 내 속도(걷기·자전거)도 보여준다. 실행 검증·스크린샷(09~11)은 `docs/speed-learning.md`.

## 추가 (2026-09-15, 이슈 #38 Google 로그인)

설정 화면 "Google 계정으로 로그인" → 서버 `/auth/config` 의 웹 클라이언트 ID 로 `google_sign_in` 7.x(Android Credential Manager) 계정 선택 → ID 토큰 → `POST /auth/google` → 서버 JWT 를 토큰 칸에 채우고 저장. 취소하면 "로그인 취소". 콘솔에 웹·Android 클라이언트 두 개가 있어야 한다([앱 README](../app/README.md)). 실행 검증은 아래 표.

| 단계 | 결과 |
|---|---|
| Compose api 재기동 | `GET /auth/config` 가 웹 클라이언트 ID(72자) 반환, `POST /auth/google` 잘못된 토큰 401, `/health` ok |
| 실기기(S23 울트라) 설정 → "Google 계정으로 로그인" | Credential Manager 계정 선택창(기기의 계정 이름·이메일이 그대로 보이는 화면이라 스크린샷은 남기지 않는다). 시스템 창이라 adb 로는 못 누르고 사용자가 직접 계정을 탭 |
| 계정 탭 뒤(1차 빌드 09:22) | 서버 로그 `POST /auth/google 200 (240ms)`, users 에 Google sub 사용자 행 생성, 앱 토큰 칸이 새 JWT 로 바뀌고 저장됨. 이 빌드는 설정 화면이 그대로 남아 뒤로가기로 나가면 홈이 옛 토큰을 쓰는 결함이 있었다(Codex 지적 → 성공 시 `_save` 처럼 pop 하도록 수정). JWT와 사용자 ID가 표시되는 화면이라 스크린샷은 공개 문서에 남기지 않는다. |
| 연결 확인(1차 빌드) | `서버 OK · 사용자 {UUID} · 내 속도 — 걷기 기본값 1.20 m/s`(새 사용자라 프로파일 없음) |
| 계정 탭 뒤(수정 빌드 10:58) | 서버 로그 `/auth/config 200 → /auth/google 200 → 10초 뒤 /places/search 200 ×4 → /routes/plan 200`, 앱 재시작 없이 새 세션으로 이어졌다. 화면은 알림창이 덮여 스크린샷을 못 남겼고, 일부러 잘못된 토큰을 넣어 두는 사전 단계는 로그에 401 이 없어 거쳤는지 확인되지 않았다 — 화면이 닫히고 홈이 새 설정을 받는 것은 위젯 테스트(`app/test/google_login_test.dart`, pop 제거 변이에서 빨강)로 보장한다 |
| 자동 검사 | backend `go test ./...`(실 DB) 통과, `flutter analyze` 0건, `flutter test` 18건 |

첫 시도에서는 계정 탭 뒤 서버 호출이 없었다(사용자가 앱을 홈으로 나간 상태였고 로그가 없어 원인 미상). 개발 토큰 사용자에 쌓인 속도 프로파일은 Google 사용자로 옮기지 않는다(다른 계정).

## 알려진 한계

- ~~카카오 역사 건물 좌표가 선로 반대편 도로에 붙는 문제~~ → 이슈 #12에서 장소명이 `…역`이면 같은 이름의 GTFS 부모역으로 앵커링해 해결했다.
- 버스·지하철 폴리라인은 정류장 사이 직선이다. 생성 GTFS 에 shapes.txt 가 없어서 OTP 가 stop-to-stop 으로 그린다. 노선 shape 는 후속.
- 에뮬레이터 `adb shell input text` 는 한글을 못 넣어 영문 검색어로 검증했다. 카카오는 영문 키워드도 한글 장소를 돌려준다. 실기기에서는 한글 입력 그대로 쓰면 된다.
- ~~로그인은 devtoken 을 설정 화면에 붙여 넣는 방식이다. Google 로그인은 GOOGLE_OAUTH_CLIENT_ID 발급 후 Phase 2b.~~ → 2026-09-15 이슈 #38: 설정 화면 "Google 계정으로 로그인"(위 "추가 (2026-09-15)" 절). devtoken 붙여 넣기는 개발용으로 남아 있다.
- ~~실기기(갤럭시 S23 울트라) 검증은 아직 안 했다.~~ → 2026-09-15 첫 실기기 검증(`docs/speed-learning.md` 실기기 절, 이슈 #36). USB 로 꽂고 `adb reverse tcp:8081 tcp:8081` 뒤 서버 주소 `http://127.0.0.1:8081`(Compose 가 loopback 에만 열려 있어 PC 내부 IP 로는 못 붙는다).
