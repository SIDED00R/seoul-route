# Phase 2a — Flutter 앱 MVP 실행 검증 (2026-09-12)

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
| 결과 목록 배지 | "N분 후 출발"(출발 대기), "실시간 ±M분"(첫 탑승 실시간 보정). 총 소요 = 출발 대기 + 소요 | 07-results-realtime.png |
| 상세 앞뒤 차·배차 | 대중교통 leg 아래 한 줄: 실시간 다음 차(첫 탑승) · 앞차/다음 열차 시각(지하철, OTP) · 배차 약 N분(버스, GTFS) | 08-detail-schedule-retina.png |
| 타일 선명도 | `TileLayer(retinaMode: true)` — 한 단계 높은 줌 타일을 절반 크기로 그려 560dpi 에서 또렷하다(지명은 작아짐) | 08-detail-schedule-retina.png |

에뮬레이터 한국어 자판 켠 뒤 adb 영문 입력은 언어 전환 키(keyevent 204)로 영문 자판으로 바꿔야 들어간다.

## 알려진 한계

- 카카오 "서울역"(역사 건물 좌표)에서 출발하면 출발점이 선로 서쪽 도로에 붙어 도보가 1.2km 늘고 421번 55분이 1순위가 된다. 70m 옆 좌표면 402번 38.9분. OSM 역 구내 연결성 문제 — 이슈 #12.
- 버스·지하철 폴리라인은 정류장 사이 직선이다. 생성 GTFS 에 shapes.txt 가 없어서 OTP 가 stop-to-stop 으로 그린다. 노선 shape 는 후속.
- 에뮬레이터 `adb shell input text` 는 한글을 못 넣어 영문 검색어로 검증했다. 카카오는 영문 키워드도 한글 장소를 돌려준다. 실기기에서는 한글 입력 그대로 쓰면 된다.
- 로그인은 devtoken 을 설정 화면에 붙여 넣는 방식이다. Google 로그인은 GOOGLE_OAUTH_CLIENT_ID 발급 후 Phase 2b.
- 실기기(갤럭시 S23 울트라) 검증은 아직 안 했다. USB 디버깅을 켜고 `cd app && flutter run` 후 설정에서 서버 주소를 PC 내부 IP 로 바꾼다.
