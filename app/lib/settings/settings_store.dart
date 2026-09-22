import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../env.dart';

// 서버 주소는 SharedPreferences에(prod flavor 는 빌드에 박힌 Env.fixedBaseUrl 만 쓴다), Google 로그인 또는 devtoken으로
// 발급한 JWT는 보안 저장소에 둔다.
class Settings {
  const Settings({required this.baseUrl, required this.token, this.voiceGuide = true});

  final String baseUrl;
  final String token;
  final bool voiceGuide; // 안내 중 음성 안내를 읽을지

  // dev flavor 기본값: 안드로이드 에뮬레이터에서 호스트 PC 의 loopback 은 10.0.2.2, 개발 스택(compose.dev.yml) api 는 8082.
  static const defaultBaseUrl = 'http://10.0.2.2:8082';

  bool get ready => baseUrl.isNotEmpty && token.isNotEmpty;
}

/// 서버 주소 정규화. 끝의 `/` 를 지운다(남기면 `//places` 처럼 붙어 chi 가 404 를 낸다). 저장·세션 양쪽이 이 함수를 쓴다.
String normalizeBaseUrl(String s) => s.trim().replaceAll(RegExp(r'/+$'), '');

class SettingsStore {
  const SettingsStore({this.tokenStorage = const SecureTokenStorage()});

  static const _kBaseUrl = 'base_url';
  static const _kLegacyToken = 'token';
  static const _kVoiceGuide = 'voice_guide';
  static const _kProdTokenPurged = 'prod_token_purged'; // prod 첫 실행 토큰 폐기를 한 번만 하기 위한 표시
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
    // prod 첫 실행 1회: flavor 가 없던 시절의 앱이 같은 패키지에 남긴 토큰(붙여 넣은 devtoken 일 수 있다)을 버린다.
    // prod 는 Google 로그인만 허용하므로 출처를 모르는 토큰은 쓰지 않는다. 홈이 "먼저 설정하세요" 를 띄워 재로그인을 이끈다.
    // 레거시 평문 토큰 수입(위) 뒤에 둬야 지운 직후 되살아나지 않는다.
    if (Env.isProd && !(p.getBool(_kProdTokenPurged) ?? false)) {
      await tokenStorage.delete();
      token = null;
      await p.setBool(_kProdTokenPurged, true);
    }
    return Settings(
      // prod 는 저장값을 무시하고 빌드에 박힌 주소만 쓴다(설정 화면에서도 못 바꾼다).
      baseUrl: Env.isProd ? Env.fixedBaseUrl : (p.getString(_kBaseUrl) ?? Settings.defaultBaseUrl),
      token: token ?? '',
      voiceGuide: p.getBool(_kVoiceGuide) ?? true,
    );
  }

  /// 음성 안내 설정만 읽는다. 안내 화면이 토큰·주소 없이 이 값만 필요할 때 쓴다. 못 읽으면 켜진 것으로 본다.
  static Future<bool> loadVoiceGuide() async {
    try {
      return (await SharedPreferences.getInstance()).getBool(_kVoiceGuide) ?? true;
    } catch (_) {
      return true;
    }
  }

  Future<void> save(Settings s) async {
    final p = await SharedPreferences.getInstance();
    await p.setString(_kBaseUrl, normalizeBaseUrl(s.baseUrl));
    await p.setBool(_kVoiceGuide, s.voiceGuide);
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
