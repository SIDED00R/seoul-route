import 'package:flutter/material.dart';
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

  testWidgets('오버레이 안내 글씨는 Material 기본 스타일을 사용한다', (tester) async {
    const dataChannel = MethodChannel('seoul_route/guide_overlay_data');
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(dataChannel, (_) async => null);
    addTearDown(
      () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(dataChannel, null),
    );

    await tester.pumpWidget(const GuideOverlayApp());
    await tester.pump();

    final context = tester.element(find.text('안내 준비 중…'));
    final style = DefaultTextStyle.of(context).style;
    expect(style.fontSize, isNot(48));
    expect(style.decoration, isNot(TextDecoration.underline));
  });
}
