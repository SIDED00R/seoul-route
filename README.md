# seoul-route

서울 안에서 대중교통·따릉이·도보를 조합하는 Android 길찾기 프로젝트입니다. Flutter 앱이 Go API를 호출하고, API가 OpenTripPlanner(OTP)의 후보에 실시간 첫 탑승 정보, 횡단보도 대기, 사용자별 이동 속도를 반영합니다.

개인용 로컬 실행을 전제로 하며 공개 배포 환경은 제공하지 않습니다.

## 구성

```text
app/       Flutter Android 앱
backend/   인증, 경로 보정, 외부 API 프록시, 궤적·속도 학습
gtfs/      서울 버스와 도시철도 데이터를 합치는 GTFS 생성기
otp/       OTP 설정과 데이터 전처리 스크립트
deploy/    PostgreSQL·OTP·API Docker Compose
docs/      모델 근거, 운영 절차, 정확도 평가
```

앱과 OTP는 API를 통해서만 통신합니다. 카카오·VWorld 등 외부 API 키는 앱에 넣지 않습니다.

## 주요 기능

- 대중교통·따릉이·도보 경로, 경유지 최대 5개, 구간별 수단 고정
- 출발 대기와 환승·대여 비용을 포함한 후보 정렬
- 서울 버스·지하철 첫 탑승 실시간 도착 보정
- 버스 배차, 지하철 앞뒤 열차, 따릉이 잔여 대수 표시
- 신호 횡단보도 기대 대기와 탑승 실패 시 재탐색
- 위치·활동 샘플 기반 개인 걷기·자전거 속도 학습
- 화면을 벗어나도 유지되는 안내 세션과 Android 위치 포그라운드 서비스
- Google 로그인 또는 개발용 JWT 인증

## 요구 환경

| 구성 요소 | 버전 |
|---|---|
| OTP | 2.10.0 |
| Java | 25 |
| Go | 1.27 |
| PostgreSQL | 17, Compose는 PostGIS 3.5 포함 |
| Flutter | 3.47.4 stable |
| Android | API 36, Android 전용 |

OTP 데이터와 그래프는 Git에 포함하지 않습니다. 그래프 빌드에는 약 8GB, 서빙에는 약 4GB의 Java heap 설정을 사용합니다.

## 설정

```powershell
Copy-Item .env.example .env
```

최소 `DATABASE_URL`과 32자 이상의 `JWT_SECRET`이 필요합니다. 선택 기능은 다음 키를 사용합니다.

| 변수 | 기능 |
|---|---|
| `SEOUL_OPENAPI_KEY` | 따릉이 GBFS |
| `DATA_GO_KR_KEY` | 서울 버스 데이터와 실시간 도착 |
| `SEOUL_SUBWAY_REALTIME_KEY` | 지하철 실시간 도착 |
| `KRIC_API_KEY` | 코레일·민자 노선 시간표 |
| `KAKAO_REST_API_KEY` | 장소 검색·역지오코딩 |
| `VWORLD_API_KEY` | 지도 타일 |
| `GOOGLE_OAUTH_CLIENT_ID` | Google 로그인 |
| `AUTH_ALLOWED_EMAILS` | Google 로그인 허용 계정(쉼표 구분). 비우면 전부 허용, 목록 밖은 403 |
| `ODSAY_API_KEY` | 경로 정확도 대조 |

값에 `$`가 있으면 Compose 보간을 막도록 작은따옴표로 감쌉니다. `.env`, API 키, OTP jar, 원천 데이터와 생성물은 커밋하지 않습니다.

## 데이터와 실행

OTP jar, South Korea OSM PBF, 국가교통DB GTFS 파일럿을 `otp/data/`에 준비한 뒤 필요한 데이터를 생성합니다.

```powershell
python otp/extract_seoul.py
python otp/extract_entrances.py
python otp/extract_crossings.py
python otp/extract_rail.py
python otp/fetch_metro_timetable.py
python otp/fetch_kric_timetable.py

Set-Location gtfs
go run ./cmd/gtfsgen fetch
go run ./cmd/gtfsgen build
Copy-Item out/seoul-gtfs.zip ../otp/data/seoul-gtfs.zip
```

원천 파일과 폴백 규칙은 [GTFS 생성기](docs/gtfs-generator.md)에 정리되어 있습니다.

OTP 그래프를 빌드하고 실행합니다.

```powershell
Set-Location ../otp
java -Xmx8G -jar otp-shaded-2.10.0.jar --build --save .
java -Xmx4G -jar otp-shaded-2.10.0.jar --load .
```

`Transit built. |Stops|=` 값이 0이면 GTFS가 빠진 그래프입니다.

백엔드는 PostgreSQL을 준비한 뒤 실행합니다.

```powershell
Set-Location ../backend
go run ./cmd/api
go run ./cmd/devtoken local-user  # Google 로그인 전 개발 토큰
```

전체 서버 스택은 루트에서 실행할 수 있습니다. 운영과 개발은 프로젝트명·DB 볼륨·포트·환경 파일이 다른 두 스택으로 나란히 돕니다.

```powershell
# 운영: .env, OTP 8080 · API 8081 (앱 prod flavor 가 붙는다)
docker compose --env-file .env -f deploy/compose.yml up -d
# 개발: .env.dev, OTP 8083 · API 8082 · DB 5433 (앱 dev flavor 가 붙는다)
docker compose --env-file .env.dev -f deploy/compose.dev.yml up -d
```

| | 운영 | 개발 |
|---|---|---|
| 파일 | `deploy/compose.yml` + `.env` | `deploy/compose.dev.yml` + `.env.dev` |
| API / OTP / DB | 8081 / 8080 / 내부만 | 8082 / 8083 / 5433 |
| `AUTH_ALLOWED_EMAILS` | 실제 개인 계정만 | 테스트 계정만 |
| `JWT_SECRET` | 서로 다른 값 | 서로 다른 값(운영 토큰이 개발에서 안 통한다) |

`.env.dev`는 `.env.example`을 한 번 더 복사해 만들고, 그래프와 `otp/data/`는 두 스택이 같은 `otp/`를 읽기 전용으로 공유합니다(`OTP_DIR`로 다른 경로 지정 가능). 개발 DB의 `seoul_route_test`는 백엔드 통합 테스트(`TEST_DATABASE_URL=postgres://seoul:seoul@localhost:5433/seoul_route_test?sslmode=disable`)에 씁니다. Compose 실행 전 `otp/graph.obj`와 `otp/data/` 생성물이 필요합니다. 모든 포트는 loopback에만 바인딩됩니다.

앱 실행 방법과 Google OAuth 설정은 [앱 README](app/README.md)를 따릅니다.

## API

| 경로 | 인증 | 설명 |
|---|---:|---|
| `GET /health` | 아니요 | DB·OTP 상태 |
| `GET /auth/config` | 아니요 | Google 웹 client ID |
| `POST /auth/google` | 아니요 | Google ID token을 서버 JWT로 교환 |
| `GET /gbfs/*.json` | 아니요 | OTP용 따릉이 GBFS |
| `POST /routes/plan` | 예 | 경로 탐색 |
| `GET`, `DELETE /routes/recent` | 예 | 최근 경로 조회·삭제 |
| `GET /places/search`, `/places/reverse` | 예 | 장소 검색·역지오코딩 |
| `GET /tiles/{z}/{x}/{y}.png` | 예 | 지도 타일 |
| `POST /trips`, `/trips/{id}/traces`, `/trips/{id}/end` | 예 | 안내 궤적과 속도 학습 |
| `GET /users/me/speed` | 예 | 개인 속도 |

## 검증

```powershell
Set-Location gtfs
go vet ./...
go test ./...

Set-Location ../backend
$env:TEST_DATABASE_URL='postgres://seoul:seoul@localhost:5432/seoul_route_test?sslmode=disable'
go vet ./...
go test ./...

Set-Location ../app
flutter analyze
flutter test
```

DB 통합 테스트는 `TEST_DATABASE_URL`이 없으면 완전한 검증이 아닙니다. 서버 변경은 `/health`, 미인증 요청, 정상 경로, 잘못된 좌표 요청의 실제 응답도 확인합니다. 앱 변경은 검색 → 결과 → 상세 → 안내 → 종료 흐름과 콘솔 오류를 에뮬레이터 또는 실기기에서 확인합니다.

## 제한과 보안

- 버스 GTFS는 평균 배차와 구간 속도에 기반한 근사값입니다.
- 일부 도시철도 노선은 원천 시간표가 없어 파일럿 데이터가 남아 있습니다.
- 원본 위치 궤적은 30일 후 삭제하고, 탈퇴 시 trip·속도 프로파일과 함께 삭제합니다.
- 앱 JWT는 Android Keystore 기반 저장소에 보관합니다.
- 로컬 개발용 평문 HTTP가 허용되어 있으므로 공개 배포 전 TLS와 Android network security 설정이 필요합니다.

## 문서

- [GTFS 생성](docs/gtfs-generator.md)
- [경로 정확도 평가](docs/routing-accuracy.md)
- [자전거 라우팅](docs/bicycle-routing.md)
- [횡단보도 대기](docs/crossing-wait.md)
- [안내·경로 이탈](docs/guide-navigation.md)
- [속도 학습](docs/speed-learning.md)
- [빠른 하차](docs/fast-exit.md)

## 라이선스

작성한 소스 코드는 [MIT License](LICENSE)를 따릅니다. 지도·교통 데이터와 스크린샷의 권리는 각 제공자에게 있습니다.
