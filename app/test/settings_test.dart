import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/screens/place_search_screen.dart';
import 'package:seoul_route/screens/home_screen.dart';
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

void main() {
  testWidgets('음성 안내 토글은 저장되고 다시 읽힌다', (tester) async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final store = SettingsStore(tokenStorage: MemoryTokenStorage());
    expect(await SettingsStore.loadVoiceGuide(), isTrue); // 설정한 적 없으면 켜짐
    await tester.pumpWidget(
      MaterialApp(
        home: SettingsScreen(
          initial: const Settings(
            baseUrl: 'http://10.0.2.2:8081',
            token: 'tok',
          ),
          settingsStore: store,
        ),
      ),
    );
    await tester.tap(find.text('음성 안내'));
    await tester.pump();
    await scrollTo(tester, find.text('저장'));
    await tester.tap(find.text('저장'));
    await tester.pumpAndSettle();
    expect((await store.load()).voiceGuide, isFalse);
    expect(await SettingsStore.loadVoiceGuide(), isFalse);
  });

  testWidgets('오버레이 권한을 허용하면 설정에 저장된다', (tester) async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    const channel = MethodChannel('seoul_route/guide_overlay');
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          if (call.method == 'requestPermission') return true;
          return null;
        });
    addTearDown(
      () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, null),
    );
    final store = SettingsStore(tokenStorage: MemoryTokenStorage());
    await tester.pumpWidget(
      MaterialApp(
        home: SettingsScreen(
          initial: const Settings(
            baseUrl: 'http://10.0.2.2:8081',
            token: 'tok',
          ),
          settingsStore: store,
        ),
      ),
    );
    await tester.tap(find.text('다른 앱 위 미니 지도'));
    await tester.pump();
    await scrollTo(tester, find.text('저장'));
    await tester.tap(find.text('저장'));
    await tester.pumpAndSettle();
    expect((await store.load()).overlayGuide, isTrue);
    expect(await SettingsStore.loadOverlayGuide(), isTrue);
  });

  testWidgets('서버 주소 끝의 / 는 저장값과 현재 세션 양쪽에서 지워진다', (tester) async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final store = SettingsStore(tokenStorage: MemoryTokenStorage());
    await tester.pumpWidget(
      MaterialApp(
        home: HomeScreen(
          settings: const Settings(
            baseUrl: 'http://10.0.2.2:8081',
            token: 'tok',
          ),
          settingsStore: store,
        ),
      ),
    );
    await tester.tap(find.byIcon(Icons.settings));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.byType(TextField).first,
      'http://10.0.2.2:8081/',
    );
    await scrollTo(tester, find.text('저장'));
    await tester.tap(find.text('저장'));
    await tester.pumpAndSettle();

    final saved = await store.load();
    expect(saved.baseUrl, 'http://10.0.2.2:8081');

    await tester.tap(find.text('출발지 선택'));
    await tester.pumpAndSettle();
    final api = tester
        .widget<PlaceSearchScreen>(find.byType(PlaceSearchScreen))
        .api;
    expect(
      api.baseUrl,
      'http://10.0.2.2:8081',
      reason: '재시작 전 세션도 정규화된 값을 써야 //places 가 안 생긴다',
    );
    expect(Uri.parse('${api.baseUrl}/places/search').path, '/places/search');
  });

  test('기존 SharedPreferences JWT를 보안 저장소로 이전하고 평문을 삭제한다', () async {
    SharedPreferences.setMockInitialValues(<String, Object>{'token': 'LEGACY'});
    final tokens = MemoryTokenStorage();
    final saved = await SettingsStore(tokenStorage: tokens).load();

    expect(saved.token, 'LEGACY');
    expect(tokens.token, 'LEGACY');
    expect(
      (await SharedPreferences.getInstance()).containsKey('token'),
      isFalse,
    );
  });

  testWidgets('연결 확인 중 화면을 나가도 폐기된 State 에서 setState 하지 않는다', (tester) async {
    await http.runWithClient(
      () async {
        await tester.pumpWidget(
          MaterialApp(
            home: Builder(
              builder: (ctx) => Scaffold(
                body: ElevatedButton(
                  onPressed: () => Navigator.push<Settings>(
                    ctx,
                    MaterialPageRoute(
                      builder: (_) => const SettingsScreen(
                        initial: Settings(
                          baseUrl: 'http://10.0.2.2:8081',
                          token: 'tok',
                        ),
                      ),
                    ),
                  ),
                  child: const Text('설정으로'),
                ),
              ),
            ),
          ),
        );
        await tester.tap(find.text('설정으로'));
        await tester.pumpAndSettle();
        await scrollTo(tester, find.text('연결 확인'));
        await tester.tap(find.text('연결 확인'));
        await tester.pump(const Duration(milliseconds: 100));
        expect(find.text('확인 중…'), findsOneWidget);

        tester.state<NavigatorState>(find.byType(Navigator)).pop();
        await tester.pumpAndSettle();
        await tester.pump(
          const Duration(seconds: 3),
        ); // 500 도착 → on ApiException → finally
        expect(tester.takeException(), isNull);
      },
      () => MockClient((req) async {
        await Future<void>.delayed(const Duration(seconds: 2));
        // charset 을 안 주면 http 패키지가 본문을 latin1 로 인코딩해 한글에서 ArgumentError 가 나고
        // 500 이 도달하지 않는다(일반 catch 로 빠짐).
        return http.Response(
          '{"error":"서버 죽음"}',
          500,
          headers: const {'content-type': 'application/json; charset=utf-8'},
        );
      }),
    );
  });
}
