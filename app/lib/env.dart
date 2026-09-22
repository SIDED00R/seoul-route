import 'package:flutter/services.dart' show appFlavor;

/// 빌드 시 고정되는 환경. Android product flavor(`--flavor dev|prod`, android/app/build.gradle.kts)와
/// `--dart-define-from-file=env.prod.json` 의 API_BASE_URL 로 정해진다.
///
/// - dev: 서버 주소를 설정 화면에서 넣고(처음엔 비어 있다) 개발용 토큰을 붙여 넣을 수 있다.
/// - prod: 서버 주소가 빌드에 박혀 바꿀 수 없고 Google 로그인만 된다. 실수로 운영 앱을 개발 서버에, 개발 앱을 운영 서버에
///   붙이는 일을 앱 쪽에서 막는다(서버 쪽은 AUTH_ALLOWED_EMAILS).
abstract final class Env {
  /// flavor 이름. 테스트·flavor 없이 실행하면 null 이라 dev 로 본다.
  static const String flavor = appFlavor ?? 'dev';

  static const bool isProd = flavor == 'prod';

  /// prod 빌드에 박힌 서버 주소. prod 인데 비어 있으면 잘못 빌드된 것이라 설정 화면이 알려 준다.
  static const String fixedBaseUrl = String.fromEnvironment('API_BASE_URL');
}
