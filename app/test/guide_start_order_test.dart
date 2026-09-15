import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/guide_screen.dart';

/// 위치 권한·서비스는 켜진 것으로 보고, 위치 스트림은 요청 설정과 구독 취소만 기록한다.
class FakeGeolocator extends GeolocatorPlatform {
  final List<LocationSettings?> streamCalls = [];
  bool cancelled = false;

  @override
  Future<LocationPermission> checkPermission() async => LocationPermission.whileInUse;

  @override
  Future<bool> isLocationServiceEnabled() async => true;

  @override
  Stream<Position> getPositionStream({LocationSettings? locationSettings}) {
    streamCalls.add(locationSettings);
    return StreamController<Position>(onCancel: () => cancelled = true).stream;
  }
}

/// trip 발급 응답을 테스트가 끝낼 때까지 붙잡아 두는 서버.
class HeldTripApi extends ApiClient {
  HeldTripApi() : super(baseUrl: 'http://x', token: 't');

  final trip = Completer<String>();
  int startCalls = 0;

  @override
  Future<String> startTrip() {
    startCalls++;
    return trip.future;
  }
}

const _leg = Leg(
  mode: 'WALK',
  durationSec: 300,
  distanceM: 400,
  fromName: 'A',
  toName: 'B',
  fromLat: 37.55,
  fromLon: 126.97,
  toLat: 37.553,
  toLon: 126.974,
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
  walkM: 400,
  legs: [_leg],
);
const _request = PlanRequest(
  origin: Place(name: 'A', address: '', lat: 37.55, lon: 126.97),
  destination: Place(name: 'B', address: '', lat: 37.553, lon: 126.974),
);

Future<void> _settle(WidgetTester tester) async {
  for (var i = 0; i < 10; i++) {
    await tester.pump();
  }
}

void main() {
  const channel = MethodChannel('seoul_route/notification_permission');

  setUp(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async => true);
  });

  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(channel, null);
  });

  testWidgets('trip 발급 응답을 기다리기 전에 위치 스트림(포그라운드 서비스 설정)을 연다', (tester) async {
    final geo = FakeGeolocator();
    GeolocatorPlatform.instance = geo;
    final api = HeldTripApi();
    await tester.pumpWidget(MaterialApp(home: GuideScreen(api: api, request: _request, itinerary: _itinerary)));
    await _settle(tester);

    expect(api.startCalls, 1);
    expect(geo.streamCalls, hasLength(1));
    expect((geo.streamCalls.single! as AndroidSettings).foregroundNotificationConfig, isNotNull);

    api.trip.complete('trip-1');
    await _settle(tester);
    expect(find.textContaining('안내 중'), findsOneWidget);
    expect(geo.cancelled, isFalse);

    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });

  testWidgets('trip 발급이 실패하면 먼저 연 위치 스트림을 닫고 실패를 보여 준다', (tester) async {
    final geo = FakeGeolocator();
    GeolocatorPlatform.instance = geo;
    final api = HeldTripApi();
    await tester.pumpWidget(MaterialApp(home: GuideScreen(api: api, request: _request, itinerary: _itinerary)));
    await _settle(tester);
    expect(geo.streamCalls, hasLength(1));

    api.trip.completeError(ApiException(500, '저장 실패'));
    await _settle(tester);
    // StreamSubscription.cancel() 의 Future 는 루트 zone 에서 끝나 가짜 시계의 pump 로는 흐르지 않는다.
    await tester.runAsync(() => Future<void>.delayed(Duration.zero));
    await _settle(tester);
    expect(geo.cancelled, isTrue);
    expect(find.textContaining('trip 발급 실패'), findsOneWidget);

    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });
}
