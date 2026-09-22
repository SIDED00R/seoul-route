# seoul-route Android 앱

Flutter 클라이언트입니다. Android만 지원하며 경로 계산, 장소 검색, 지도 타일, 로그인은 루트의 Go API를 사용합니다.

## 실행

요구 환경은 Flutter 3.47.4, Android API 36, Gradle JDK 21입니다.

```powershell
flutter pub get
flutter analyze
flutter test
flutter run --flavor dev
```

## 개발계·운영 분리 (flavor)

앱은 Android product flavor 두 개로 빌드합니다. `--flavor`를 빼면 Gradle이 빌드를 거부합니다.

| | `dev` | `prod` |
|---|---|---|
| 패키지 · 이름 | `kr.seoulroute.seoul_route.dev` · 서울 길찾기 Dev | `kr.seoulroute.seoul_route` · 서울 길찾기 |
| 붙는 서버 | 개발 스택(`deploy/compose.dev.yml`, 8082). 설정 화면에서 바꿀 수 있음 | 빌드에 박힌 `API_BASE_URL`(운영 스택 8081). 설정 화면은 읽기 전용 |
| 인증 | Google 로그인 또는 devtoken 붙여넣기 | Google 로그인만. 첫 실행 때 이전 앱이 남긴 토큰을 버리므로 한 번 다시 로그인한다 |
| 실행 | `flutter run --flavor dev` | `flutter build apk --flavor prod --dart-define-from-file=env.prod.json` |

두 앱은 패키지가 달라 한 폰에 같이 설치됩니다. `env.prod.json`은 `env.prod.json.example`을 복사해 운영 API 주소(Tailscale 호스트명 URL)를 넣고, 커밋하지 않습니다. `API_BASE_URL` 없이 prod를 빌드하면 설정 화면이 "빌드에 서버 주소가 없습니다"라고 알립니다.

dev flavor 의 API 주소:

| 환경 | API 주소 |
|---|---|
| Android 에뮬레이터 | `http://10.0.2.2:8082` (기본값) |
| USB 실기기 | `adb reverse tcp:8082 tcp:8082` 후 `http://127.0.0.1:8082` |
| Tailscale | PC에서 개발 API를 serve한 tailnet 호스트명 URL |

Compose는 API를 `127.0.0.1`에만 공개하므로 같은 Wi-Fi의 사설 IP로 직접 연결할 수 없습니다.

## Google 로그인

같은 Google Cloud 프로젝트에 다음 OAuth client가 필요합니다.

1. 웹 client ID를 루트 `.env`(운영)와 `.env.dev`(개발)의 `GOOGLE_OAUTH_CLIENT_ID`에 설정합니다(같은 값이어도 됩니다).
2. Android client를 flavor 마다 만듭니다: 운영 `kr.seoulroute.seoul_route`, 개발 `kr.seoulroute.seoul_route.dev`. 둘 다 앱 서명 SHA-1이 필요합니다.
3. 동의 화면이 테스트 상태라면 사용할 계정(실제 계정과 테스트 계정)을 테스트 사용자에 추가합니다.
4. 서버의 `AUTH_ALLOWED_EMAILS`에 운영은 실제 계정, 개발은 테스트 계정만 넣고 API를 재시작합니다. 목록 밖 계정은 로그인 403.

디버그 서명 SHA-1은 다음 명령으로 확인합니다.

```powershell
keytool -list -v -keystore $env:USERPROFILE\.android\debug.keystore -alias androiddebugkey -storepass android
```

Google 로그인 없이 개발할 때는 `backend`에서 `go run ./cmd/devtoken local-user`로 JWT를 발급합니다.

## Android 권한

| 권한 | 용도 |
|---|---|
| 위치 | 현재 위치, 안내 추적, 속도 학습 |
| foreground service/location | 백그라운드 위치 수집 |
| 알림 | 안내 포그라운드 서비스 표시 |
| 활동 인식 | 수단과 맞지 않는 샘플 제외 |
| wake lock | 안내 위치 스트림 유지 |

알림이나 활동 인식 권한을 거부해도 안내는 계속되지만 관련 표시·필터가 빠집니다. 위치 권한이나 위치 서비스가 없으면 안내를 시작할 수 없습니다.

## 구조

```text
lib/api/        HTTP client
lib/auth/       Google 로그인
lib/guide/      안내 세션, 위치·구간·정차 추적, 음성, 궤적 업로드
lib/location/   현재 위치 선택
lib/models/     API 요청·응답 모델
lib/screens/    홈, 검색, 결과, 상세, 안내, 설정
lib/settings/   API 주소와 JWT 저장
lib/util/       공통 변환
lib/widgets/    공통 UI
test/           모델·화면·안내 회귀 테스트
```

## 실행 검증

prod flavor 전용 테스트(첫 실행 토큰 폐기·고정 주소)는 기본 `flutter test`에서 skip되므로 따로 돌립니다.

```powershell
flutter test --flavor prod --dart-define=API_BASE_URL=http://prod.example:8081 test/prod_flavor_test.dart
```

1. 설정 화면에서 `/health`, `/users/me`, `/users/me/speed` 연결 확인
2. 장소 검색 → 경로 목록 → 지도 상세 진입
3. 안내 시작 후 위치·알림·활동 권한 확인
4. 화면을 끄거나 홈으로 이동한 뒤에도 샘플 업로드 확인
5. 안내 종료 후 남은 샘플 전송과 속도 프로파일 확인
6. `flutter run`과 `adb logcat -s flutter:E AndroidRuntime:E`에서 예외 확인

에뮬레이터 위치는 `adb emu geo fix <longitude> <latitude>`로 주입할 수 있습니다.

앱은 로컬 사용을 위해 평문 HTTP와 debug key release 서명을 사용합니다. 공개 배포 전 TLS, network security, release signing 설정이 필요합니다.
