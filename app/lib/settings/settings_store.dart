import 'package:shared_preferences/shared_preferences.dart';

// 서버 주소와 토큰을 기기에 저장한다. 개발 단계는 devtoken 으로 만든 JWT 를 직접 붙여 넣는다.
class Settings {
  const Settings({required this.baseUrl, required this.token});

  final String baseUrl;
  final String token;

  // 안드로이드 에뮬레이터에서 호스트 PC 의 loopback 은 10.0.2.2 다.
  static const defaultBaseUrl = 'http://10.0.2.2:8081';

  bool get ready => baseUrl.isNotEmpty && token.isNotEmpty;
}

/// 서버 주소 정규화. 끝의 `/` 를 지운다(남기면 `//places` 처럼 붙어 chi 가 404 를 낸다). 저장·세션 양쪽이 이 함수를 쓴다.
String normalizeBaseUrl(String s) => s.trim().replaceAll(RegExp(r'/+$'), '');

class SettingsStore {
  static const _kBaseUrl = 'base_url';
  static const _kToken = 'token';

  Future<Settings> load() async {
    final p = await SharedPreferences.getInstance();
    return Settings(
      baseUrl: p.getString(_kBaseUrl) ?? Settings.defaultBaseUrl,
      token: p.getString(_kToken) ?? '',
    );
  }

  Future<void> save(Settings s) async {
    final p = await SharedPreferences.getInstance();
    await p.setString(_kBaseUrl, normalizeBaseUrl(s.baseUrl));
    await p.setString(_kToken, s.token.trim());
  }
}
