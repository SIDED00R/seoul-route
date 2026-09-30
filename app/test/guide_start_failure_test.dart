import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/guide/active_guide.dart';
import 'package:seoul_route/guide/guide_session.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/detail_screen.dart';

/// 위치 권한과 위치 서비스 상태를 테스트가 정한다.
class _Geo extends GeolocatorPlatform {
  LocationPermission permission = LocationPermission.whileInUse;
  bool serviceOn = true;
  int streamCalls = 0;
  final controller = StreamController<Position>.broadcast();

  @override
  Future<LocationPermission> checkPermission() async => permission;

  @override
  Future<LocationPermission> requestPermission() async => permission;

  @override
  Future<bool> isLocationServiceEnabled() async => serviceOn;

  @override
  Stream<Position> getPositionStream({LocationSettings? locationSettings}) {
    streamCalls++;
    return controller.stream;
  }
}

/// trip 발급을 failFirst 번까지 실패시키는 서버.
class _Api extends ApiClient {
  _Api() : super(baseUrl: 'http://x', token: 't');

  int failFirst = 0;
  int startCalls = 0;
  Completer<void>? startGate;

  @override
  Future<String> startTrip() async {
    startCalls++;
    await startGate?.future;
    if (startCalls <= failFirst) throw ApiException(500, '저장 실패');
    return 'T$startCalls';
  }

  @override
  Future<void> uploadTraces(String tripId, List samples) async {}

  @override
  Future<Map<String, dynamic>> endTrip(String tripId) async => const {};
}

const _leg = Leg(
  mode: 'WALK',
  durationSec: 300,
  distanceM: 222,
  fromName: 'Origin',
  toName: 'Destination',
  fromLat: 37.5,
  fromLon: 127.0,
  toLat: 37.502,
  toLon: 127.0,
  route: '',
  rentedBike: false,
  transitLeg: false,
  polyline: '',
  start: '2026-09-15T09:00:00+09:00',
  end: '2026-09-15T09:05:00+09:00',
);
const _itinerary = Itinerary(
  start: '2026-09-15T09:00:00+09:00',
  end: '2026-09-15T09:05:00+09:00',
  durationSec: 300,
  transfers: 0,
  walkM: 222,
  legs: [_leg],
);
const _request = PlanRequest(
  origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착', address: '', lat: 37.502, lon: 127.0),
);

Future<void> _settle(WidgetTester tester) async {
  for (var i = 0; i < 10; i++) {
    await tester.pump(const Duration(milliseconds: 10));
  }
}

// 시작하지 못한 안내는 끝난 세션이고, 다시 시작하면 새 세션이 된다.
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const permission = MethodChannel('seoul_route/notification_permission');
  const status = MethodChannel('seoul_route/guide_status');
  late _Geo geo;
  late _Api api;
  late List<String> statusCalls;

  setUp(() {
    ActiveGuide.instance.clear();
    SharedPreferences.setMockInitialValues(<String, Object>{});
    statusCalls = [];
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(permission, (call) async => true);
    messenger.setMockMethodCallHandler(status, (call) async {
      statusCalls.add(call.method);
      return null;
    });
    geo = _Geo();
    GeolocatorPlatform.instance = geo;
    api = _Api();
  });

  tearDown(() {
    ActiveGuide.instance.clear();
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(permission, null);
    messenger.setMockMethodCallHandler(status, null);
  });

  GuideSession newSession() {
    final s = GuideSession(api: api, request: _request, itinerary: _itinerary, speak: (_) async {});
    ActiveGuide.instance.set(s);
    return s;
  }

  for (final p in [LocationPermission.denied, LocationPermission.deniedForever]) {
    test('위치 권한이 없으면(${p.name}) 이유를 남기고 끝난 세션이 된다', () async {
      geo.permission = p;
      final s = newSession();
      await s.start();
      expect(s.ended, isTrue);
      expect(s.status, contains('위치 권한이 없어'));
      expect(geo.streamCalls, 0);
      expect(api.startCalls, 0);
    });
  }

  test('위치 서비스가 꺼져 있으면 이유를 남기고 끝난 세션이 된다', () async {
    geo.serviceOn = false;
    final s = newSession();
    await s.start();
    expect(s.ended, isTrue);
    expect(s.status, '기기 위치 서비스가 꺼져 있습니다.');
    expect(geo.streamCalls, 0);
    expect(api.startCalls, 0);
  });

  test('trip 발급이 실패하면 이유를 남기고 끝난 세션이 된다', () async {
    api.failFirst = 1;
    final s = newSession();
    await s.start();
    expect(s.ended, isTrue);
    expect(s.status, contains('trip 발급 실패'));
    expect(s.uploader, isNull);
  });

  test('trip 발급을 기다리는 동안 위치 표본이 띄운 진행 알림은 발급이 실패하면 지운다', () async {
    api.failFirst = 1;
    api.startGate = Completer<void>();
    final s = newSession();
    final started = s.start();
    await Future<void>.delayed(Duration.zero);
    geo.controller.add(Position(
      latitude: 37.5, longitude: 127.0, timestamp: DateTime.now(), accuracy: 5, altitude: 0,
      altitudeAccuracy: 0, heading: 0, headingAccuracy: 0, speed: 0, speedAccuracy: 0,
    ));
    await Future<void>.delayed(Duration.zero);
    expect(statusCalls, ['show']);
    api.startGate!.complete();
    await started;
    expect(s.ended, isTrue);
    expect(statusCalls.last, 'cancel');
  });

  testWidgets('trip 발급 실패로 시작하지 못한 안내와 같은 여정을 상세 화면에서 다시 시작하면 새 세션으로 시작한다', (tester) async {
    api.failFirst = 1;
    final failed = newSession();
    // 위치 스트림 구독 취소는 가짜 시계 pump 로 끝나지 않는다.
    await tester.runAsync(failed.start);
    expect(api.startCalls, 1);

    await tester.pumpWidget(MaterialApp(
      home: DetailScreen(api: api, request: _request, itinerary: _itinerary, index: 0),
    ));
    await tester.pump();
    await tester.tap(find.text('안내 시작'));
    await tester.pumpAndSettle();
    await _settle(tester);
    expect(api.startCalls, 2);
    final retried = ActiveGuide.instance.current!;
    expect(retried, isNot(same(failed)));
    expect(retried.ended, isFalse);
    expect(find.textContaining('안내 중'), findsOneWidget);

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });
}
