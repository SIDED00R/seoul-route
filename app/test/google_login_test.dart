import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/auth/google_login.dart';
import 'package:seoul_route/screens/settings_screen.dart';
import 'package:seoul_route/settings/settings_store.dart';

import 'scroll_to.dart';

class MemoryTokenStorage implements TokenStorage {
  String? token;

  @override
  Future<String?> read() async => token;

  @override
  Future<void> write(String value) async => token = value;

  @override
  Future<void> delete() async => token = null;
}

/// 로그인 성공은 저장만 하면 안 되고 _save 처럼 새 설정을 돌려주며 화면을 닫아야 한다 — 홈(PlanScreen)은 push 결과로만
/// 세션을 갱신하므로, 안 돌려주면 앱을 다시 켤 때까지 옛 토큰(또는 빈 토큰)을 쓴다.
void main() {
  // push 결과 Future 는 out 에 담아 준다. 반환값으로 넘기면 async 가 그 Future 를 기다려 취소 케이스(화면이 안 닫힘)가 끝나지 않는다.
  Future<void> openAndLogin(
      WidgetTester tester, Future<LoginResult?> Function(dynamic) login, List<Future<Settings?>> out,
      SettingsStore settingsStore) async {
    await tester.pumpWidget(MaterialApp(
      home: Builder(
        builder: (ctx) => ElevatedButton(
          onPressed: () => out.add(Navigator.push<Settings>(
            ctx,
            MaterialPageRoute(
              builder: (_) => SettingsScreen(
                initial: const Settings(baseUrl: 'http://10.0.2.2:8081', token: 'OLD'),
                login: (api) => login(api),
                settingsStore: settingsStore,
              ),
            ),
          )),
          child: const Text('open'),
        ),
      ),
    ));
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Google 계정으로 로그인'));
    await tester.pumpAndSettle();
  }

  testWidgets('로그인 성공: 토큰을 저장하고 새 설정을 돌려주며 닫힌다', (tester) async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final store = SettingsStore(tokenStorage: MemoryTokenStorage());
    final out = <Future<Settings?>>[];
    await openAndLogin(tester, (_) async => const LoginResult(token: 'NEWJWT', userId: 'u1'), out, store);
    expect(find.byType(SettingsScreen), findsNothing, reason: '성공하면 설정 화면이 닫혀야 한다');
    expect((await out.single)?.token, 'NEWJWT', reason: '홈이 push 결과로 세션을 갱신한다');
    expect((await store.load()).token, 'NEWJWT');
  });

  testWidgets('로그인 취소: 화면에 남고 토큰은 그대로', (tester) async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final store = SettingsStore(tokenStorage: MemoryTokenStorage());
    await openAndLogin(tester, (_) async => null, <Future<Settings?>>[], store);
    expect(find.byType(SettingsScreen), findsOneWidget);
    await scrollTo(tester, find.text('로그인 취소')); // 상태 문구는 목록 맨 아래(화면 밖)
    expect(find.text('로그인 취소'), findsOneWidget);
    expect((await store.load()).token, '');
  });
}
