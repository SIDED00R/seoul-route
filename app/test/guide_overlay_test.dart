import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:latlong2/latlong.dart';

import 'package:seoul_route/guide/guide_overlay.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('오버레이 스냅샷은 지도·안내 정보를 보존한다', () {
    const original = GuideOverlaySnapshot(
      baseUrl: 'http://server',
      token: 'secret',
      points: [LatLng(37.5, 127.0), LatLng(37.501, 127.001)],
      here: LatLng(37.5, 127.0),
      turn: LatLng(37.501, 127.001),
      now: '80m 직진 후 우회전',
      next: '지하철 탑승',
      remainMin: 12,
      eta: '09:30',
      color: 0xFF00695C,
    );
    final restored = GuideOverlaySnapshot.fromMap(original.toMap());
    expect(restored.baseUrl, original.baseUrl);
    expect(restored.points, original.points);
    expect(restored.here, original.here);
    expect(restored.turn, original.turn);
    expect(restored.now, original.now);
    expect(restored.remainMin, 12);
  });

  test('오버레이 권한 요청과 끄기 명령을 네이티브 채널로 보낸다', () async {
    const channel = MethodChannel('seoul_route/guide_overlay');
    final calls = <MethodCall>[];
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          calls.add(call);
          if (call.method == 'requestPermission') return true;
          return null;
        });
    addTearDown(
      () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, null),
    );
    expect(await GuideOverlayPlatform.requestPermission(), isTrue);
    await GuideOverlayPlatform.stop();
    expect(calls.map((c) => c.method), ['requestPermission', 'setEnabled']);
    expect((calls.last.arguments as Map)['enabled'], isFalse);
  });

  testWidgets('네이티브가 보낸 update 스냅샷이 오버레이 본문(안내 문구·다음·남은 시간)으로 그려진다', (tester) async {
    const dataChannel = MethodChannel('seoul_route/guide_overlay_data');
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(dataChannel, (_) async => null); // 오버레이 엔진의 'ready' 호출 수신
    addTearDown(() => messenger.setMockMethodCallHandler(dataChannel, null));

    await tester.pumpWidget(const GuideOverlayApp());
    await tester.pump();
    expect(find.text('안내 준비 중…'), findsOneWidget);

    // 네이티브 → 오버레이 엔진 방향 호출을 흉내 낸다(base_url 이 비어 타일 요청은 나가지 않는다).
    final snapshot = const GuideOverlaySnapshot(
      baseUrl: '',
      token: '',
      points: [LatLng(37.5665, 126.978), LatLng(37.5670, 126.979)],
      here: LatLng(37.5665, 126.978),
      turn: LatLng(37.5670, 126.979),
      now: '80m 직진 후 우회전',
      next: '탑승 · 버스 402',
      remainMin: 12,
      eta: '09:30',
      color: 0xFF1565C0,
    ).toMap();
    await messenger.handlePlatformMessage(
      dataChannel.name,
      const StandardMethodCodec().encodeMethodCall(MethodCall('update', snapshot)),
      (_) {},
    );
    await tester.pump();

    expect(find.text('안내 준비 중…'), findsNothing);
    expect(find.text('80m 직진 후 우회전'), findsOneWidget);
    expect(find.text('다음: 탑승 · 버스 402'), findsOneWidget);
    expect(find.text('12분\n09:30'), findsOneWidget);
  });
}
