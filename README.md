# seoul-route

서울 안에서 대중교통·따릉이·도보를 한 여정으로 조합하는 Android 길찾기 프로젝트입니다. OpenTripPlanner(OTP)의 후보를 Go API가 결합·보정하고, 사용자별 실측 이동 속도와 신호 횡단보도 기대 대기, 첫 탑승 실시간 도착정보를 반영합니다.

현재 상태는 개인용 로컬 실행 버전입니다. 앱과 전체 스택은 Android 실기기·에뮬레이터에서 검증했지만, 공개 서비스로 배포하지 않았습니다.

## 실행 화면

아래 화면은 실제 로컬 API·OTP에 연결해 실행한 결과입니다. 전체 검증 기록은 [앱 실행 보고서](docs/app-phase2.md)와 [속도 학습·백그라운드 수집](docs/speed-learning.md)에 있습니다.

<table>
  <tr>
    <td align="center"><img src="docs/app/11-current-home.png" width="220" alt="Galaxy S23 Ultra에서 실행한 최신 홈 화면"><br>최신 실기기 홈 화면</td>
    <td align="center"><img src="docs/app/03-results.png" width="220" alt="멀티모달 경로 후보 목록"><br>경로 후보와 실시간 배지</td>
    <td align="center"><img src="docs/app/04-detail-bus.png" width="220" alt="지도 위에 표시된 경로 상세"><br>지도·구간 상세</td>
    <td align="center"><img src="docs/app/10-guide.png" width="220" alt="현재 구간과 위치를 표시하는 안내 화면"><br>실시간 안내·궤적 수집</td>
  </tr>
</table>

## 주요 기능

- 대중교통·따릉이·도보 후보를 함께 탐색하고 출발 대기까지 포함해 정렬
- 경유지 최대 5개와 구간별 수단 고정
- 서울 버스·지하철 첫 탑승 실시간 도착 보정
- 지하철 앞뒤 열차와 버스 배차간격 표시
- OSM 신호 횡단보도 기대 대기 반영과 탑승 실패 시 1회 재탐색
- `…역` 장소를 가까운 GTFS 부모역에 연결해 역사 좌표 스냅 오류 완화
- 안내 중 위치·활동 샘플을 모아 개인 걷기·자전거 속도 학습
- Android 포그라운드 서비스로 화면이 꺼진 동안에도 안내 샘플 수집
- Google 로그인 또는 개발용 JWT 인증

## 구성

```text
app/       Flutter Android 앱
backend/   Go API: 인증, 경로 결합·보정, 장소/타일 프록시, 궤적·속도 학습
gtfs/      서울 버스 API와 도시철도 시간표를 합치는 GTFS 생성기
otp/       OTP 2.10 설정, OSM/시간표 전처리 스크립트, 스파이크 도구
deploy/    PostgreSQL·OTP·API 로컬 Docker Compose
docs/      설계 근거, 실행 검증, 경로 정확도 평가
```

요청 흐름은 `Flutter → Go API → OTP`입니다. API는 카카오 장소 검색과 VWorld 타일도 프록시하므로 외부 API 키를 앱에 넣지 않습니다. OTP는 OSM 도로망, 생성 GTFS, API가 제공하는 따릉이 GBFS를 사용합니다.

## 요구 환경

| 구성 요소 | 확인한 버전 |
|---|---|
| OTP | 2.10.0 |
| Java | 25(OTP class file 69) |
| Go | 1.27 |
| PostgreSQL | 17 + PostGIS 3.5(Compose) |
| Flutter | 3.47.4 stable |
| Android | SDK/API 36, Android 전용 |

전체 그래프 데이터는 Git에 포함하지 않습니다. 최종 그래프 빌드는 약 4.1GB, 서빙은 약 2.2GB RSS가 측정되어 메모리 여유가 필요합니다.

## 빠른 시작

### 1. 환경변수

```powershell
Copy-Item .env.example .env
```

`.env`에 최소 `DATABASE_URL`과 32자 이상의 `JWT_SECRET`을 채웁니다. 주요 선택 변수는 다음과 같습니다.
API 키에 `$`가 들어가면 Docker Compose가 변수로 해석하지 않도록 값을 작은따옴표로 감싸세요.

| 변수 | 용도 |
|---|---|
| `SEOUL_OPENAPI_KEY` | 따릉이 `bikeList` → GBFS |
| `DATA_GO_KR_KEY` | 서울 버스 도착정보와 GTFS 버스 데이터 수집 |
| `SEOUL_SUBWAY_REALTIME_KEY` | 지하철 첫 탑승 실시간 보정 |
| `KRIC_API_KEY` | 코레일·민자 노선 요일별 시간표 수집 |
| `KAKAO_REST_API_KEY` | 장소 검색 |
| `VWORLD_API_KEY` | 지도 타일 |
| `GOOGLE_OAUTH_CLIENT_ID` | 앱 Google 로그인용 웹 클라이언트 ID |
| `ODSAY_API_KEY` | 선택적 경로 정확도 대조 |

`.env`, API 키, OTP jar, 다운로드·생성 데이터, 그래프, GTFS 캐시는 Git에서 제외됩니다.

### 2. OTP 데이터 준비

1. OTP 2.10.0 shaded jar를 `otp/otp-shaded-2.10.0.jar`에 둡니다.
2. Geofabrik의 South Korea OSM PBF를 `otp/data/south-korea-latest.osm.pbf`에 둡니다.
3. 국가교통DB GTFS 파일럿을 `otp/data/202503_GTFS_DataSet/`에 풉니다.
4. 필요한 산출물을 생성합니다.

```powershell
python otp/extract_seoul.py
python otp/extract_entrances.py
python otp/extract_crossings.py
python otp/fetch_metro_timetable.py
python otp/fetch_kric_timetable.py

Set-Location gtfs
go run ./cmd/gtfsgen fetch
go run ./cmd/gtfsgen build
Copy-Item out/seoul-gtfs.zip ../otp/data/seoul-gtfs.zip
Set-Location ..
```

원천 데이터별 발급·폴백 규칙과 생성 결과는 [GTFS 생성기 문서](docs/gtfs-generator.md)에 정리되어 있습니다.

### 3. OTP 그래프 빌드

```powershell
Set-Location otp
java -Xmx8G -jar otp-shaded-2.10.0.jar --build --save .
java -Xmx4G -jar otp-shaded-2.10.0.jar --load .
```

서빙 주소는 `http://localhost:8080/otp/gtfs/v1`입니다. 빌드 로그의 `Transit built. |Stops|=`가 0이 아닌지 확인해야 합니다.

### 4. 백엔드 실행

PostgreSQL을 준비한 뒤:

```powershell
Set-Location backend
go run ./cmd/api
```

API는 `http://localhost:8081`에서 열리고 기동 시 SQL migration을 적용합니다. Google 로그인 전에는 다음 명령으로 개발용 JWT를 만들 수 있습니다.

```powershell
go run ./cmd/devtoken local-user
```

### 5. 전체 스택을 Compose로 실행

루트에서 Docker Desktop WSL2 엔진을 켠 뒤 실행합니다.

```powershell
docker compose --env-file .env -f deploy/compose.yml up -d
docker compose --env-file .env -f deploy/compose.yml ps
```

`otp/graph.obj`, `otp/data/seoul-gtfs.zip`, `otp/data/crossings.csv`가 먼저 있어야 합니다. 호스트 포트 8080·8081은 loopback에만 공개됩니다.

### 6. Android 앱 실행

```powershell
Set-Location app
flutter pub get
flutter run
```

- Android 에뮬레이터 서버 주소: `http://10.0.2.2:8081`
- USB 실기기: `adb reverse tcp:8081 tcp:8081` 후 `http://127.0.0.1:8081`
- Google 로그인 설정과 권한 설명: [앱 README](app/README.md)

## API 요약

| 경로 | 인증 | 설명 |
|---|---:|---|
| `GET /health` | 아니요 | DB·OTP 연결 확인 |
| `GET /auth/config` | 아니요 | Google 웹 클라이언트 ID |
| `POST /auth/google` | 아니요 | Google ID token → 서버 JWT |
| `GET /gbfs/*.json` | 아니요 | OTP용 따릉이 GBFS 2.3 |
| `POST /routes/plan` | 예 | 멀티모달 경로 탐색 |
| `GET /places/search` | 예 | 카카오 장소 검색 프록시 |
| `GET /tiles/{z}/{x}/{y}.png` | 예 | VWorld 타일 프록시 |
| `POST /trips` | 예 | 안내 trip 시작 |
| `POST /trips/{id}/traces` | 예 | 위치·활동 샘플 업로드 |
| `POST /trips/{id}/end` | 예 | trip 종료와 속도 학습 |
| `GET /users/me/speed` | 예 | 개인 이동 속도 조회 |

## 검증

```powershell
# GTFS 생성기
Set-Location gtfs
go vet ./...
go test ./...

# 백엔드: TEST_DATABASE_URL이 없으면 DB 통합 테스트는 skip됨
Set-Location ../backend
$env:TEST_DATABASE_URL='postgres://seoul:seoul@localhost:5432/seoul_route_test?sslmode=disable'
go vet ./...
go test ./...

# Flutter
Set-Location ../app
flutter analyze
flutter test
```

테스트만으로 기능 완료를 판정하지 않습니다. OTP·API를 띄운 뒤 정상 경로 요청과 잘못된 좌표 요청을 보내 응답 본문을 확인하고, 앱에서는 검색 → 목록 → 상세 → 안내 → 종료 흐름을 실제 기기나 에뮬레이터에서 실행해야 합니다. 대표 OD 비교 방법은 [경로 정확도 문서](docs/routing-accuracy.md)에 있습니다.

### 최근 통합 검증

2026-09-16에 Docker Compose로 PostgreSQL·OTP·API를 실제 기동하고 다음을 재검증했습니다.

- `/health`: DB `ok`, OTP `ok`
- 실제 OTP 서울역 → 강남역 대중교통 요청: 후보 1개, 4개 구간 반환
- 미인증 `/routes/plan`: `401`
- PostgreSQL을 사용하는 `internal/httpapi` 통합 테스트 전체 통과
- Flutter 정적 분석 및 38개 테스트 통과
- Galaxy S23 Ultra에 디버그 APK 재설치·실행, Android 오류 로그 없음
- 기존 SharedPreferences JWT가 Android Keystore 보안 저장소로 이전되고 평문 키가 삭제됨

## 데이터·보안 정책

- 원본 위치 궤적은 30일 후 삭제하며, 탈퇴 시 trip·속도 프로파일과 함께 즉시 삭제합니다.
- 외부 API 키와 JWT secret은 서버 `.env`에만 둡니다.
- 앱 JWT는 Android Keystore 기반 보안 저장소에 암호화하며, 이전 평문 저장값은 첫 실행 때 이전·삭제합니다.
- 앱은 현재 로컬 개발을 위해 평문 HTTP를 허용합니다. 공개 배포 전에는 TLS와 Android network security 설정을 적용해야 합니다.
- 버스 GTFS는 평균 배차와 구간 속도를 기반으로 한 근사이며 실제 운행 시각표가 아닙니다.
- 서해선 일부 연장 구간과 GTX-A는 요일별 원천 시간표가 없어 파일럿 데이터가 남아 있습니다.
- 생성 GTFS에 `shapes.txt`가 없어 버스·지하철 폴리라인은 정류장 사이 직선입니다.

## 문서

- [OTP·외부 API 스파이크](docs/spike-report.md)
- [GTFS 생성과 출입구·환승 통로](docs/gtfs-generator.md)
- [경로 정확도 평가](docs/routing-accuracy.md)
- [횡단보도 대기 모델](docs/crossing-wait.md)
- [안내 궤적과 속도 학습](docs/speed-learning.md)
- [Flutter 앱 실행 검증](docs/app-phase2.md)

## 개발 규칙

변경은 Issue → branch → PR → review → merge 순서로 진행합니다. 커밋 메시지와 PR 본문에는 AI trailer를 넣지 않습니다.
