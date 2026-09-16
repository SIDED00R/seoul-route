# seoul-route Android 앱

`app/`은 서울 멀티모달 길찾기의 Flutter 클라이언트입니다. Android만 지원하며, 경로 계산·장소 검색·지도 타일·로그인은 모두 루트의 Go API를 통해 처리합니다.

## 제공 기능

- 출발지·도착지 검색과 현재 위치 사용
- 경유지 최대 5개, 구간별 전체/도보/따릉이/대중교통 고정
- 경로 후보, 출발 대기, 실시간 보정, 횡단보도 대기 표시
- VWorld 지도 위 구간별 폴리라인과 대여·반납 지점 표시
- 현재 구간 자동 전환과 수동 이전/다음 구간 이동
- 화면이 꺼져도 유지되는 Android 위치 포그라운드 서비스
- 활동 인식으로 안내 수단과 다른 위치 샘플 제외
- Google 로그인과 개발용 JWT 입력

실행 화면은 [루트 README](../README.md#실행-화면), 실제 기기 검증 기록은 [앱 실행 보고서](../docs/app-phase2.md)와 [속도 학습 문서](../docs/speed-learning.md)에 있습니다.

## 요구 환경

- Flutter 3.47.4 stable
- Dart SDK `^3.13.3`
- Android SDK/API 36
- Gradle JDK 21
- 실행 중인 seoul-route API와 OTP

## 실행

```powershell
flutter pub get
flutter analyze
flutter test
flutter run
```

앱 설정 화면에서 API 주소와 인증 정보를 넣습니다.

| 실행 환경 | 서버 주소 |
|---|---|
| Android 에뮬레이터 | `http://10.0.2.2:8081` |
| USB 실기기 | `adb reverse tcp:8081 tcp:8081` 후 `http://127.0.0.1:8081` |
| Tailscale | PC에서 API를 serve한 뒤 tailnet 호스트명 URL 사용 |

Compose의 API 포트는 `127.0.0.1`에만 바인딩되어 있으므로 같은 Wi-Fi의 사설 IP로 직접 연결되지 않습니다.

## Google 로그인 설정

같은 Google Cloud 프로젝트에 OAuth client 두 개가 필요합니다.

1. 웹 client ID를 루트 `.env`의 `GOOGLE_OAUTH_CLIENT_ID`에 설정합니다.
2. Android client를 패키지 `kr.seoulroute.seoul_route`와 앱 서명 SHA-1로 만듭니다.
3. OAuth 동의 화면이 테스트 상태라면 사용할 계정을 테스트 사용자에 추가합니다.
4. API를 재시작하고 설정 화면에서 `Google 계정으로 로그인`을 누릅니다.

디버그 서명 SHA-1 확인 예시:

```powershell
keytool -list -v -keystore $env:USERPROFILE\.android\debug.keystore -alias androiddebugkey -storepass android
```

Google 로그인을 설정하지 않은 로컬 개발에서는 `backend`에서 `go run ./cmd/devtoken local-user`로 JWT를 발급해 토큰 칸에 붙여 넣을 수 있습니다.

## Android 권한

| 권한 | 사용 이유 |
|---|---|
| 위치(정확/대략) | 현재 위치, 안내 구간 추적, 속도 학습 |
| foreground service/location | 앱이 백그라운드이거나 화면이 꺼진 동안 위치 수집 |
| 알림 | Android 13+에서 안내 포그라운드 서비스 알림 표시 |
| 활동 인식 | 걷기·자전거·차량 판정으로 잘못 분류된 샘플 제외 |
| wake lock | 안내 위치 스트림 유지 |

알림 또는 활동 인식 권한을 거부해도 안내 자체는 계속되지만 관련 표시·필터가 빠집니다. 위치 권한이나 위치 서비스가 없으면 안내를 시작할 수 없습니다.

## 디렉터리

```text
lib/api/        HTTP client
lib/auth/       Google ID token 로그인
lib/guide/      구간 추적, 활동 판정, 위치 업로드, 종료 흐름
lib/location/   현재 위치 선택
lib/models/     API 요청·응답 모델
lib/screens/    홈, 검색, 결과, 상세, 안내, 설정
lib/settings/   API 주소와 JWT 저장
lib/util/       폴리라인·표시 이름 변환
lib/widgets/    수단 아이콘·색상
test/           모델·화면·안내 회귀 테스트
```

## 실행 검증 체크리스트

1. 설정에서 `/health`, `/users/me`, `/users/me/speed` 연결 확인
2. 장소 검색 → 경로 목록 → 지도 상세 진입
3. 안내 시작 후 위치·알림·활동 권한 동작 확인
4. 화면 끄기 또는 홈 전환 중 샘플 업로드 지속 확인
5. 안내 종료 후 남은 샘플 전송과 속도 프로파일 응답 확인
6. `flutter run` 및 `adb logcat -s flutter:E AndroidRuntime:E`에 예외가 없는지 확인
7. 이전 버전에서 올렸다면 `shared_preferences`의 평문 `token` 키가 없어지고 보안 저장소에 JWT가 남는지 확인

에뮬레이터에서는 다음 형식으로 위치를 주입할 수 있습니다.

```powershell
adb emu geo fix <longitude> <latitude>
```

## 현재 제한

- 앱은 개인 로컬 사용을 전제로 평문 HTTP를 허용합니다.
- JWT는 Android Keystore 기반 보안 저장소에 암호화해 저장합니다. 이전 버전의 `shared_preferences` 토큰은 첫 실행 때 자동 이전·삭제합니다.
- 앱 프로세스가 강제 종료되면 위치 포그라운드 서비스와 샘플 수집도 끝납니다.
- 생성 GTFS에 `shapes.txt`가 없어 대중교통 경로선은 정류장 사이 직선입니다.
