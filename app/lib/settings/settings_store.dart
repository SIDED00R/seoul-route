import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:shared_preferences/shared_preferences.dart';

// 서버 주소는 SharedPreferences에, Google 로그인 또는 devtoken으로 발급한 JWT는 보안 저장소에 둔다.
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
  const SettingsStore({this.tokenStorage = const SecureTokenStorage()});

  static const _kBaseUrl = 'base_url';
  static const _kLegacyToken = 'token';
  final TokenStorage tokenStorage;

  Future<Settings> load() async {
    final p = await SharedPreferences.getInstance();
    var token = await tokenStorage.read();
    final legacyToken = p.getString(_kLegacyToken);
    if ((token == null || token.isEmpty) && legacyToken != null && legacyToken.isNotEmpty) {
      await tokenStorage.write(legacyToken);
      token = legacyToken;
    }
    if (legacyToken != null) await p.remove(_kLegacyToken);
    return Settings(
      baseUrl: p.getString(_kBaseUrl) ?? Settings.defaultBaseUrl,
      token: token ?? '',
    );
  }

  Future<void> save(Settings s) async {
    final p = await SharedPreferences.getInstance();
    await p.setString(_kBaseUrl, normalizeBaseUrl(s.baseUrl));
    final token = s.token.trim();
    if (token.isEmpty) {
      await tokenStorage.delete();
    } else {
      await tokenStorage.write(token);
    }
    await p.remove(_kLegacyToken);
  }
}

abstract interface class TokenStorage {
  Future<String?> read();
  Future<void> write(String token);
  Future<void> delete();
}

/// Android에서는 Keystore로 감싼 키와 AES-GCM으로 JWT를 암호화한다.
final class SecureTokenStorage implements TokenStorage {
  const SecureTokenStorage();

  static const _key = 'jwt';
  static const _storage = FlutterSecureStorage(aOptions: AndroidOptions());

  @override
  Future<String?> read() => _storage.read(key: _key);

  @override
  Future<void> write(String token) => _storage.write(key: _key, value: token);

  @override
  Future<void> delete() => _storage.delete(key: _key);
}
