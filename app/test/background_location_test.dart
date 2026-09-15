import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';

import 'package:seoul_route/guide/background_location.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('안내 위치 스트림은 포그라운드 서비스 알림과 wake lock 을 켜고 주어진 간격으로 받는다', () {
    final s = guideLocationSettings(const Duration(seconds: 5));
    expect(s, isA<AndroidSettings>());
    final a = s as AndroidSettings;
    expect(a.intervalDuration, const Duration(seconds: 5));
    expect(a.accuracy, LocationAccuracy.high);
    expect(a.distanceFilter, 0);
    final fg = a.foregroundNotificationConfig;
    expect(fg, isNotNull);
    expect(fg!.enableWakeLock, isTrue);
    expect(fg.setOngoing, isTrue);
    expect(a.toJson()['foregroundNotificationConfig'], isNotNull);
  });

  test('알림 권한 요청은 네이티브 응답을 돌려주고, 채널이 없으면 false', () async {
    const channel = MethodChannel('seoul_route/notification_permission');
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    final calls = <String>[];
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call.method);
      return true;
    });
    expect(await requestNotificationPermission(), isTrue);
    expect(calls, ['request']);

    messenger.setMockMethodCallHandler(channel, (call) async => false);
    expect(await requestNotificationPermission(), isFalse);

    messenger.setMockMethodCallHandler(channel, null);
    expect(await requestNotificationPermission(), isFalse);
  });
}
