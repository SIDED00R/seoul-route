import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/env.dart';
import 'package:seoul_route/settings/settings_store.dart';

import 'settings_test.dart' show MemoryTokenStorage;

// prod flavor 전용. Env.isProd 는 빌드 상수라 이 파일만 따로 돌린다(기본 실행에서는 skip):
//   flutter test --flavor prod --dart-define=API_BASE_URL=http://prod.example:8081 test/prod_flavor_test.dart
void main() {
  const fixed = 'http://prod.example:8081';

  test('prod 는 저장된 서버 주소를 무시하고 빌드에 박힌 주소만 쓴다', () async {
    SharedPreferences.setMockInitialValues(<String, Object>{'base_url': 'http://10.0.2.2:8082'});
    final s = await SettingsStore(tokenStorage: MemoryTokenStorage()).load();
    expect(s.baseUrl, fixed);
  }, skip: !Env.isProd);

  test('prod 첫 실행은 이전 앱이 남긴 토큰(보안 저장소·레거시 평문)을 버리고, Google 로그인으로 받은 토큰은 유지한다', () async {
    SharedPreferences.setMockInitialValues(<String, Object>{'token': 'legacy-plain'});
    final storage = MemoryTokenStorage()..token = 'old-devtoken';
    final store = SettingsStore(tokenStorage: storage);

    final first = await store.load();
    expect(first.token, '', reason: '첫 실행에 남은 토큰이 쓰이면 안 된다');
    expect(storage.token, isNull, reason: '보안 저장소의 토큰이 지워져야 한다');
    expect((await SharedPreferences.getInstance()).getString('token'), isNull, reason: '레거시 평문 토큰도 남지 않는다');

    // Google 로그인이 저장한 토큰은 다음 실행에도 남는다(폐기는 1회).
    await store.save(const Settings(baseUrl: fixed, token: 'google-issued'));
    final second = await store.load();
    expect(second.token, 'google-issued');
  }, skip: !Env.isProd);
}
