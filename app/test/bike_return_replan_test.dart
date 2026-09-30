import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:latlong2/latlong.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/guide/active_guide.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/guide_screen.dart';

import 'support/polyline_encode.dart';

class _Geo extends GeolocatorPlatform {
  final controller = StreamController<Position>.broadcast();

  @override
  Future<LocationPermission> checkPermission() async => LocationPermission.whileInUse;

  @override
  Future<bool> isLocationServiceEnabled() async => true;

  @override
  Stream<Position> getPositionStream({LocationSettings? locationSettings}) => controller.stream;
}

class _Api extends ApiClient {
  _Api() : super(baseUrl: 'http://x', token: 't');

  final List<PlanRequest> plans = [];
  PlanResult reply = const PlanResult(itineraries: []);

  @override
  Future<String> startTrip() async => 'T1';

  @override
  Future<void> uploadTraces(String tripId, List samples) async {}

  @override
  Future<Map<String, dynamic>> endTrip(String tripId) async => const {};

  @override
  Future<PlanResult> plan(PlanRequest req) async {
    plans.add(req);
    return reply;
  }
}

Position _pos(double lat, double lon, {double speed = 0, double acc = 8}) => Position(
      latitude: lat,
      longitude: lon,
      timestamp: DateTime.now(),
      accuracy: acc,
      altitude: 0,
      altitudeAccuracy: 0,
      heading: 0,
      headingAccuracy: 0,
      speed: speed,
      speedAccuracy: 0,
    );

final _base = DateTime.now();
String _t(int m) => _base.add(Duration(minutes: m)).toIso8601String();

Leg _leg(String mode, String from, String to, double fromLat, double toLat, int start, int end,
        {bool rented = false, bool transit = false}) =>
    Leg(
      mode: mode,
      durationSec: (end - start) * 60.0,
      distanceM: (toLat - fromLat) * 111195,
      fromName: from,
      toName: to,
      fromLat: fromLat,
      fromLon: 127.0,
      toLat: toLat,
      toLon: 127.0,
      route: transit ? '146' : '',
      rentedBike: rented,
      transitLeg: transit,
      polyline: encodePolyline([LatLng(fromLat, 127.0), LatLng(toLat, 127.0)]),
      start: _t(start),
      end: _t(end),
    );

// 도보(37.5→37.501) → 따릉이(→37.505 대여소) → 도보(→37.506 정류장) → 버스(→37.52 도착지). 위도 0.001도 ≈ 111m.
final _itinerary = Itinerary(
  start: _t(0),
  end: _t(30),
  durationSec: 1800,
  transfers: 0,
  walkM: 222,
  legs: [
    _leg('WALK', '출발', '대여소A', 37.5, 37.501, 0, 2),
    _leg('BICYCLE', '대여소A', '대여소B', 37.501, 37.505, 2, 8, rented: true),
    _leg('WALK', '대여소B', '정류장', 37.505, 37.506, 8, 10),
    _leg('BUS', '정류장', '도착지', 37.506, 37.52, 10, 30, transit: true),
  ],
);

// 반납한 곳(37.502, 127.002)에서 정류장까지 걷는 새 경로.
final _fresh = encodePolyline([const LatLng(37.502, 127.002), const LatLng(37.506, 127.0)]);
final _freshReply = PlanResult(itineraries: [
  Itinerary(
    start: _t(5),
    end: _t(12),
    durationSec: 420,
    transfers: 0,
    walkM: 500,
    legs: [
      Leg(
        mode: 'WALK',
        durationSec: 420,
        distanceM: 500,
        fromName: '현재 위치',
        toName: '정류장',
        fromLat: 37.502,
        fromLon: 127.002,
        toLat: 37.506,
        toLon: 127.0,
        route: '',
        rentedBike: false,
        transitLeg: false,
        polyline: _fresh,
        start: _t(5),
        end: _t(12),
      ),
    ],
  ),
]);

const _request = PlanRequest(
  origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착지', address: '', lat: 37.52, lon: 127.0),
);

Future<void> _settle(WidgetTester tester) async {
  for (var i = 0; i < 10; i++) {
    await tester.pump();
  }
}

late DateTime clock;

void main() {
  const channel = MethodChannel('seoul_route/notification_permission');
  late _Geo geo;
  late _Api api;
  late List<String> spoken;

  setUp(() {
    ActiveGuide.instance.clear();
    SharedPreferences.setMockInitialValues(<String, Object>{});
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async => true);
    geo = _Geo();
    GeolocatorPlatform.instance = geo;
    api = _Api();
    spoken = [];
    clock = _base;
  });

  tearDown(() {
    ActiveGuide.instance.clear();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(channel, null);
  });

  Future<void> push(WidgetTester tester, Position p) async {
    clock = clock.add(const Duration(seconds: 2));
    geo.controller.add(p);
    await _settle(tester);
  }

  /// 안내를 띄우고 따릉이 구간(index 1)까지 간다.
  Future<void> pumpOnBike(WidgetTester tester) async {
    await tester.pumpWidget(MaterialApp(
      home: GuideScreen(
        api: api,
        request: _request,
        itinerary: _itinerary,
        speak: (t) async => spoken.add(t),
        clock: () => clock,
      ),
    ));
    await _settle(tester);
    await push(tester, _pos(37.5, 127.0));
    expect(find.text('다른 대여소에 반납'), findsNothing); // 도보 구간에는 없다
    await push(tester, _pos(37.501, 127.0));
    expect(ActiveGuide.instance.current!.tracker.index, 1);
  }

  Future<void> leave(WidgetTester tester) async {
    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  }

  /// 경로선에서 동쪽으로 176m, 대여소B 에서 330m 넘게 떨어진 자리에서 북쪽으로 걷는 표본을 n 개(2초 간격) 넣는다.
  Future<void> walkAway(WidgetTester tester, int n,
      {double lon = 127.002, double lat0 = 37.502, double speed = 1.2}) async {
    for (var i = 0; i < n; i++) {
      await push(tester, _pos(lat0 + i * 2 * speed / 111195, lon, speed: speed));
    }
  }

  testWidgets('"다른 대여소에 반납" 을 누르면 자전거 구간과 뒤 도보 구간을 걷는 경로로 갈아 끼운다', (tester) async {
    api.reply = _freshReply;
    await pumpOnBike(tester);
    await push(tester, _pos(37.502, 127.002));
    await tester.tap(find.text('다른 대여소에 반납'));
    await tester.pumpAndSettle();

    final req = api.plans.single;
    expect(req.origin.name, '현재 위치');
    expect((req.origin.lat, req.origin.lon), (37.502, 127.002));
    expect(req.destination.name, '정류장');
    expect(req.segmentModes, [SegmentMode.walk]);
    final tracker = ActiveGuide.instance.current!.tracker;
    expect(tracker.legs.map((l) => l.mode), ['WALK', 'WALK', 'BUS']);
    expect(tracker.index, 1);
    expect(tracker.current.polyline, _fresh);
    expect(spoken, contains('따릉이를 반납한 곳에서 걸어가는 경로로 다시 안내합니다'));
    expect(find.text('다른 대여소에 반납'), findsNothing);
    await leave(tester);
  });

  testWidgets('대여소·경로선에서 떨어져 90초 넘게 걷는 속도로 움직이면 스스로 갈아 끼운다', (tester) async {
    api.reply = _freshReply;
    await pumpOnBike(tester);
    await walkAway(tester, 45); // 88초
    expect(api.plans, isEmpty);
    await walkAway(tester, 2, lat0: 37.502 + 45 * 2 * 1.2 / 111195);
    await tester.pumpAndSettle();

    expect(api.plans.length, 1);
    expect(api.plans.single.destination.name, '정류장');
    expect(ActiveGuide.instance.current!.tracker.legs.map((l) => l.mode), ['WALK', 'WALK', 'BUS']);
    await leave(tester);
  });

  testWidgets('계획 경로선 위를 느리게 가거나(끌고 가기) 자전거 속도면 갈아 끼우지 않는다', (tester) async {
    api.reply = _freshReply;
    await pumpOnBike(tester);
    await walkAway(tester, 60, lon: 127.0); // 경로선 위
    await walkAway(tester, 60, speed: 4.0); // 자전거 속도
    await walkAway(tester, 60, lon: 127.001, lat0: 37.5045); // 대여소B 150m 안(경로선에서는 88m)
    await tester.pumpAndSettle();
    expect(api.plans, isEmpty);
    expect(ActiveGuide.instance.current!.tracker.index, 1);
    await leave(tester);
  });

  testWidgets('걷는 경로를 못 찾으면 원래 구간으로 이어 간다', (tester) async {
    await pumpOnBike(tester);
    await push(tester, _pos(37.502, 127.002));
    await tester.tap(find.text('다른 대여소에 반납'));
    await tester.pumpAndSettle();

    expect(api.plans.length, 1);
    final tracker = ActiveGuide.instance.current!.tracker;
    expect(tracker.legs.length, 4);
    expect(tracker.index, 1);
    expect(find.text('다른 대여소에 반납'), findsOneWidget);
    await leave(tester);
  });
}
