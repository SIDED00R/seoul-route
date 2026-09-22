import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/guide/guide_overlay.dart';
import 'package:seoul_route/screens/settings_screen.dart';
import 'package:seoul_route/settings/settings_store.dart';

import 'scroll_to.dart';
import 'settings_test.dart' show MemoryTokenStorage;

// 미니 지도 진하기(overlayOpacity): 저장·복원·상한 자르기, 슬라이더가 네이티브에 바로 반영, 안내 시작 시 전달.
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('불투명도는 저장·복원되고 범위 밖 값은 상한·하한으로 잘린다', () async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final store = SettingsStore(tokenStorage: MemoryTokenStorage());
    expect((await store.load()).overlayOpacity, Settings.defaultOverlayOpacity);
    expect(await SettingsStore.loadOverlayOpacity(), Settings.defaultOverlayOpacity);

    await store.save(const Settings(baseUrl: 'http://x', token: 't', overlayOpacity: 0.35));
    expect((await store.load()).overlayOpacity, closeTo(0.35, 1e-9));
    expect(await SettingsStore.loadOverlayOpacity(), closeTo(0.35, 1e-9));

    // 0.8 을 넘기면 Android 가 뒤 앱 터치를 막으므로 저장 단계에서 자른다.
    await store.save(const Settings(baseUrl: 'http://x', token: 't', overlayOpacity: 0.95));
    expect((await store.load()).overlayOpacity, Settings.maxOverlayOpacity);
    await store.save(const Settings(baseUrl: 'http://x', token: 't', overlayOpacity: 0.0));
    expect((await store.load()).overlayOpacity, Settings.minOverlayOpacity);
  });

  testWidgets('슬라이더를 놓으면 setOpacity 로 바로 반영되고, 저장하면 값이 남는다', (tester) async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final calls = <MethodCall>[];
    const channel = MethodChannel('seoul_route/guide_overlay');
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call);
      return null;
    });
    addTearDown(() => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(channel, null));
    final store = SettingsStore(tokenStorage: MemoryTokenStorage());
    await tester.pumpWidget(MaterialApp(
      home: SettingsScreen(
        initial: const Settings(baseUrl: 'http://x', token: 't', overlayGuide: true, overlayOpacity: 0.5),
        settingsStore: store,
      ),
    ));
    expect(find.text('미니 지도 진하기 50%'), findsOneWidget);

    // 슬라이더를 오른쪽 끝(상한)으로 끈다.
    final slider = find.byType(Slider);
    await tester.drag(slider, const Offset(600, 0));
    await tester.pumpAndSettle();
    expect(find.text('미니 지도 진하기 80%'), findsOneWidget);
    expect(calls.map((c) => c.method), ['setOpacity']);
    expect((calls.single.arguments as Map)['opacity'], closeTo(0.8, 1e-9));

    await scrollTo(tester, find.text('저장'));
    await tester.tap(find.text('저장'));
    await tester.pumpAndSettle();
    expect((await store.load()).overlayOpacity, closeTo(0.8, 1e-9));
  });

  testWidgets('네이티브 손잡이 슬라이더가 보낸 opacityChanged 는 설정에 저장되고, 설정 화면이 그 값을 보여 준다', (tester) async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    const channel = MethodChannel('seoul_route/guide_overlay');
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(channel, (_) async => null);
    addTearDown(() => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(channel, null));
    await GuideOverlayPlatform.setEnabled(true, opacity: 0.5); // 이때 앱→네이티브 핸들러가 걸린다
    await tester.binding.defaultBinaryMessenger.handlePlatformMessage(
      channel.name,
      const StandardMethodCodec().encodeMethodCall(const MethodCall('opacityChanged', 0.3)),
      (_) {},
    );
    expect(await SettingsStore.loadOverlayOpacity(), closeTo(0.3, 1e-9));

    await tester.pumpWidget(MaterialApp(
      home: SettingsScreen(
        initial: const Settings(baseUrl: 'http://x', token: 't', overlayGuide: true, overlayOpacity: 0.5),
        settingsStore: SettingsStore(tokenStorage: MemoryTokenStorage()),
      ),
    ));
    await tester.pumpAndSettle();
    expect(find.text('미니 지도 진하기 30%'), findsOneWidget, reason: 'initial(0.5)보다 저장소 값이 우선');
  });

  testWidgets('미니 지도가 꺼져 있으면 슬라이더는 비활성이다', (tester) async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    await tester.pumpWidget(MaterialApp(
      home: SettingsScreen(
        initial: const Settings(baseUrl: 'http://x', token: 't', overlayGuide: false),
        settingsStore: SettingsStore(tokenStorage: MemoryTokenStorage()),
      ),
    ));
    expect(tester.widget<Slider>(find.byType(Slider)).onChanged, isNull);
  });
}
