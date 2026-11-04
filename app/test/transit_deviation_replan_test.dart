import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:latlong2/latlong.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/guide/active_guide.dart';
import 'package:seoul_route/guide/guide_session.dart';
import 'package:seoul_route/guide/guide_store.dart';
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

Position _pos(double lat, double lon) => Position(
      latitude: lat,
      longitude: lon,
      timestamp: DateTime.now(),
      accuracy: 8,
      altitude: 0,
      altitudeAccuracy: 0,
      heading: 0,
      headingAccuracy: 0,
      speed: 0,
      speedAccuracy: 0,
    );

final _base = DateTime.now();
String _t(int m) => _base.add(Duration(minutes: m)).toIso8601String();

// 합성 좌표. 지하철은 37.5 를 따라 동쪽으로 약 4.4km(탑승 127.0 → 하차 127.05), 내려서 북쪽 도착지까지 걷는다.
Leg _leg(String mode, double fromLat, double fromLon, double toLat, double toLon, int start, int end,
        {String route = ''}) =>
    Leg(
      mode: mode,
      durationSec: (end - start) * 60.0,
      distanceM: 0,
      fromName: '출발',
      toName: 'Destination',
      fromLat: fromLat,
      fromLon: fromLon,
      toLat: toLat,
      toLon: toLon,
      route: route,
      rentedBike: false,
      transitLeg: mode != 'WALK',
      polyline: encodePolyline([LatLng(fromLat, fromLon), LatLng(toLat, toLon)]),
      start: _t(start),
      end: _t(end),
    );

final _itinerary = Itinerary(
  start: _t(0),
  end: _t(15),
  durationSec: 900,
  transfers: 0,
  walkM: 222,
  legs: [
    _leg('SUBWAY', 37.5, 127.0, 37.5, 127.05, 0, 12, route: '2호선'),
    _leg('WALK', 37.5, 127.05, 37.502, 127.05, 12, 15),
  ],
);

const _request = PlanRequest(
  origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착지', address: '', lat: 37.502, lon: 127.05),
);

Itinerary _fresh(List<Leg> legs, int endMin) =>
    Itinerary(start: _t(0), end: _t(endMin), durationSec: endMin * 60.0, transfers: 0, walkM: 0, legs: legs);

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

  Future<void> pump(WidgetTester tester, {PlanRequest request = _request}) async {
    await tester.pumpWidget(MaterialApp(
      home: GuideScreen(
        api: api,
        request: request,
        itinerary: _itinerary,
        speak: (t) async => spoken.add(t),
        clock: () => clock,
      ),
    ));
    await _settle(tester);
  }

  /// sec 초 뒤 위치를 넣는다.
  Future<void> at(WidgetTester tester, int sec, double lat, double lon) async {
    clock = _base.add(Duration(seconds: sec));
    geo.controller.add(_pos(lat, lon));
    await _settle(tester);
  }

  /// 중간 역(탑승역에서 약 1.8km)에서 내려 20초마다 동쪽으로 25m 씩 걷는다(지하철 바로 위 길).
  Future<void> walkAfterAlighting(WidgetTester tester) async {
    for (var i = 0; i <= 5; i++) {
      await at(tester, 300 + i * 20, 37.5, 127.02 + i * 0.000284);
    }
    await tester.pumpAndSettle();
  }

  Future<void> close(WidgetTester tester) async {
    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  }

  testWidgets('다른 역에서 내려 걸으면 지금 위치에서 목적지까지 다시 찾아 남은 구간을 갈아 끼운다', (tester) async {
    // 서버 응답처럼 출발 이름이 "Origin" 인 구간
    final walk = Leg.fromJson({
      'mode': 'WALK',
      'duration_sec': 2040,
      'distance_m': 2400,
      'from_name': 'Origin',
      'to_name': 'Destination',
      'from_lat': 37.5,
      'from_lon': 127.0214,
      'to_lat': 37.502,
      'to_lon': 127.05,
      'polyline': encodePolyline([const LatLng(37.5, 127.0214), const LatLng(37.502, 127.05)]),
      'start': _t(6),
      'end': _t(40),
    });
    api.reply = PlanResult(itineraries: [_fresh([walk], 40)]);
    await pump(tester);
    expect(ActiveGuide.instance.current!.tracker.current.mode, 'SUBWAY');

    await walkAfterAlighting(tester);

    final s = ActiveGuide.instance.current!;
    final req = api.plans.single;
    expect(req.origin.name, '현재 위치');
    expect(req.originOnBoard, isFalse);
    expect(req.destination.name, '도착지');
    // 타고 온 지하철 구간은 마친 것으로 남고 그 뒤를 갈아 끼운다.
    expect(s.tracker.legs.map((l) => l.mode), ['SUBWAY', 'WALK']);
    expect(s.tracker.index, 1);
    expect(s.tracker.current.fromName, '현재 위치'); // "Origin" 을 도착지 이름으로 바꿔 보여 주지 않는다
    expect(s.tracker.shift, Duration.zero);
    expect(s.eta, DateTime.parse(_t(40)));
    expect(spoken, contains('지하철에서 내린 것 같아 지금 위치에서 경로를 다시 찾았습니다'));
    // 앱이 죽었다 다시 켜져도 다시 찾은 여정의 도착 시각으로 이어받는다.
    final saved = jsonDecode((await SharedPreferences.getInstance()).getString('active_guide')!) as Map;
    expect(DateTime.parse(saved['eta'] as String), DateTime.parse(_t(40)));
    expect(((saved['legs'] as List).last as Map)['from_name'], '현재 위치');
    expect(saved['leg_index'], 1);
    await close(tester);
  });

  testWidgets('경로선에서 멀리 열차 속도로 가면 열차 안 출발로 다시 찾고 그 역에서 타는 여정을 고른다', (tester) async {
    final walkFirst = _leg('WALK', 37.507, 127.02, 37.502, 127.05, 5, 30);
    final ride = _leg('SUBWAY', 37.507, 127.02, 37.503, 127.05, 5, 15, route: '5호선');
    api.reply = PlanResult(itineraries: [
      _fresh([walkFirst], 30),
      _fresh([ride, _leg('WALK', 37.503, 127.05, 37.502, 127.05, 15, 17)], 17),
    ]);
    await pump(tester);

    await at(tester, 60, 37.5, 127.01); // 경로선 위(탑승역 근처)
    await at(tester, 180, 37.507, 127.02); // 북동쪽 1.2km 역, 경로선에서 약 780m
    expect(api.plans, isEmpty);
    await at(tester, 186, 37.507, 127.02); // 같은 역에서 한 번 더
    await tester.pumpAndSettle();

    final s = ActiveGuide.instance.current!;
    final req = api.plans.single;
    expect(req.originOnBoard, isTrue);
    expect(req.toJson()['origin'], containsPair('on_board', true));
    expect(s.tracker.current.route, '5호선');
    expect(s.tracker.legs.length, 2);
    expect(s.tracker.index, 0); // 계획한 지하철 구간부터 갈아 끼운다
    expect(s.eta, DateTime.parse(_t(17)));
    expect(spoken, contains('계획과 다른 열차로 가고 있는 것 같아 경로를 다시 찾았습니다'));
    await close(tester);
  });

  testWidgets('하차역에서 구간이 넘어간 뒤에도 열차로 계속 가면 도보가 아니라 열차 안 출발로 다시 찾는다', (tester) async {
    final back = _leg('SUBWAY', 37.5, 127.062, 37.5, 127.05, 5, 7, route: '2호선');
    api.reply = PlanResult(itineraries: [
      _fresh([back, _leg('WALK', 37.5, 127.05, 37.502, 127.05, 7, 10)], 10),
    ]);
    await pump(tester);

    await at(tester, 600, 37.5, 127.0499); // 하차역 정차(40m 안) — 내려서 걷는 구간으로 넘어간다
    expect(ActiveGuide.instance.current!.tracker.current.mode, 'WALK');
    // 내리지 않고 다음 역(동쪽 약 1km)까지 갔다
    await at(tester, 690, 37.5, 127.062);
    await at(tester, 696, 37.5, 127.062);
    await tester.pumpAndSettle();

    final s = ActiveGuide.instance.current!;
    expect(api.plans.single.originOnBoard, isTrue);
    expect(s.tracker.legs.map((l) => l.mode), ['SUBWAY', 'SUBWAY', 'WALK']);
    expect(s.tracker.index, 1);
    await close(tester);
  });

  testWidgets('계획 경로에서 먼 자리에서 안내를 이어받으면 걷는 표본을 다른 열차로 보지 않고 도보 이탈로 다시 찾는다', (tester) async {
    // 지하철에서 내려서 걷는 구간(하차역→도착지)에서 남긴 안내
    final snap = GuideSnapshot(
      tripId: 'T1',
      request: _request,
      itinerary: _itinerary,
      legs: _itinerary.legs,
      legIndex: 1,
      shift: Duration.zero,
      startedAt: _base,
    );
    final s = GuideSession.resume(snap, api: api, speak: (t) async => spoken.add(t));
    ActiveGuide.instance.set(s);
    unawaited(s.start());
    await tester.pumpWidget(MaterialApp(home: GuideScreen.resume(s)));
    await _settle(tester);
    // 계획 경로(지하철 경로선·도보 구간)에서 북쪽으로 약 560m 떨어진 곳을 걷는다
    for (var i = 0; i < 6; i++) {
      geo.controller.add(_pos(37.507 + i * 0.00002, 127.05));
      await _settle(tester);
    }
    await tester.pumpAndSettle();

    expect(api.plans.where((r) => r.originOnBoard), isEmpty);
    expect(api.plans.single.segmentModes, [SegmentMode.walk]);
    await close(tester);
  });

  testWidgets('역에서 먼 곳에서 다음 구간을 눌러 지하철 구간으로 옮기면 그 자리 표본을 다른 열차로 보지 않는다', (tester) async {
    final walkFirst = Itinerary(
      start: _t(0),
      end: _t(25),
      durationSec: 1500,
      transfers: 0,
      walkM: 1100,
      legs: [
        _leg('WALK', 37.49, 127.0, 37.5, 127.0, 0, 13),
        _leg('SUBWAY', 37.5, 127.0, 37.5, 127.05, 13, 22, route: '2호선'),
        _leg('WALK', 37.5, 127.05, 37.502, 127.05, 22, 25),
      ],
    );
    await tester.pumpWidget(MaterialApp(
      home: GuideScreen(api: api, request: _request, itinerary: walkFirst, speak: (t) async {}, clock: () => clock),
    ));
    await _settle(tester);
    await at(tester, 10, 37.49, 127.0); // 역에서 약 1.1km, 첫 도보 구간 시작점
    ActiveGuide.instance.current!.nextLeg();
    await _settle(tester);
    expect(ActiveGuide.instance.current!.tracker.current.mode, 'SUBWAY');
    await at(tester, 12, 37.49, 127.0);
    await at(tester, 14, 37.49, 127.0);
    await tester.pumpAndSettle();

    expect(api.plans, isEmpty);
    await close(tester);
  });

  testWidgets('역에서 먼 곳에서 이전 구간을 눌러 지하철 구간으로 되돌리면 그 자리 표본을 다른 열차로 보지 않는다', (tester) async {
    await pump(tester);
    ActiveGuide.instance.current!.nextLeg(); // 내려서 걷는 구간으로
    await _settle(tester);
    await at(tester, 600, 37.507, 127.05); // 탑승역에서 약 4.4km, 계획 경로에서 약 560m
    ActiveGuide.instance.current!.prevLeg();
    await _settle(tester);
    expect(ActiveGuide.instance.current!.tracker.current.mode, 'SUBWAY');
    await at(tester, 602, 37.507, 127.05);
    await at(tester, 604, 37.507, 127.05);
    await tester.pumpAndSettle();

    expect(api.plans, isEmpty);
    await close(tester);
  });

  testWidgets('역 안 환승 통로에서는 지하 위치가 흔들려도 도보 이탈로 다시 찾지 않는다', (tester) async {
    final transfer = Itinerary(
      start: _t(0),
      end: _t(20),
      durationSec: 1200,
      transfers: 1,
      walkM: 100,
      legs: [
        _leg('SUBWAY', 37.5, 127.0, 37.5, 127.02, 0, 8, route: '4호선'),
        Leg(
          mode: 'WALK',
          durationSec: 180,
          distanceM: 100,
          fromName: '환승역',
          toName: '환승역',
          fromLat: 37.5,
          fromLon: 127.02,
          toLat: 37.5009,
          toLon: 127.02,
          route: '',
          rentedBike: false,
          transitLeg: false,
          inStation: true,
          polyline: encodePolyline([const LatLng(37.5, 127.02), const LatLng(37.5009, 127.02)]),
          start: _t(8),
          end: _t(11),
        ),
        _leg('SUBWAY', 37.5009, 127.02, 37.5009, 127.06, 11, 20, route: '2호선'),
      ],
    );
    await tester.pumpWidget(MaterialApp(
      home: GuideScreen(api: api, request: _request, itinerary: transfer, speak: (t) async {}, clock: () => clock),
    ));
    await _settle(tester);
    await at(tester, 480, 37.5, 127.0199); // 환승역 도착 — 환승 통로로 넘어간다
    expect(ActiveGuide.instance.current!.tracker.current.inStation, isTrue);
    // 통로에서 동쪽으로 약 120m 떨어진 자리가 6초마다 여덟 번(도보 이탈 기준은 60m 5회)
    for (var i = 1; i <= 8; i++) {
      await at(tester, 480 + i * 6, 37.5004, 127.0214);
    }
    await tester.pumpAndSettle();

    expect(api.plans, isEmpty);
    expect(ActiveGuide.instance.current!.tracker.current.inStation, isTrue);
    await close(tester);
  });

  testWidgets('다시 찾은 경로가 없으면 원래 경로로 이어 간다', (tester) async {
    await pump(tester);
    await walkAfterAlighting(tester);

    final s = ActiveGuide.instance.current!;
    expect(api.plans.length, 1);
    expect(s.tracker.current.mode, 'SUBWAY');
    expect(s.tracker.legs.length, 2);
    expect(s.eta, DateTime.parse(_t(15)));
    await close(tester);
  });

  testWidgets('경유지가 있는 안내는 다시 찾지 않는다', (tester) async {
    const withVia = PlanRequest(
      origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
      destination: Place(name: '도착지', address: '', lat: 37.502, lon: 127.05),
      via: [Place(name: '경유', address: '', lat: 37.5, lon: 127.03)],
    );
    await pump(tester, request: withVia);
    await walkAfterAlighting(tester);

    expect(api.plans, isEmpty);
    expect(ActiveGuide.instance.current!.tracker.current.mode, 'SUBWAY');
    await close(tester);
  });
}
