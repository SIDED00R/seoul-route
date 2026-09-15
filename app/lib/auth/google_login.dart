import 'package:google_sign_in/google_sign_in.dart';

import '../api/client.dart';

/// Google 계정으로 로그인해 서버 JWT 를 받는다: 서버의 /auth/config 에서 웹 클라이언트 ID → Google 계정 선택(Credential
/// Manager) → ID 토큰 → POST /auth/google. 사용자가 계정 선택을 취소하면 null.
/// 안드로이드에서는 같은 Google Cloud 프로젝트에 이 앱의 패키지명·서명 SHA-1 로 만든 Android 클라이언트도 있어야 한다
/// (없으면 GoogleSignInException 으로 실패한다).
class GoogleLogin {
  const GoogleLogin(this.api);

  final ApiClient api; // token 은 비어 있어도 된다(무인증 경로만 쓴다)

  Future<LoginResult?> signIn() async {
    final clientId = await api.googleClientId();
    if (clientId.isEmpty) {
      throw StateError('서버에 Google 로그인이 설정돼 있지 않습니다(GOOGLE_OAUTH_CLIENT_ID)');
    }
    final google = GoogleSignIn.instance;
    await google.initialize(serverClientId: clientId);
    final GoogleSignInAccount account;
    try {
      account = await google.authenticate();
    } on GoogleSignInException catch (e) {
      if (e.code == GoogleSignInExceptionCode.canceled) return null;
      rethrow;
    }
    final idToken = account.authentication.idToken;
    if (idToken == null) {
      throw StateError('Google 이 ID 토큰을 주지 않았습니다');
    }
    return api.loginGoogle(idToken);
  }
}

class LoginResult {
  const LoginResult({required this.token, required this.userId});

  final String token;
  final String userId;
}
