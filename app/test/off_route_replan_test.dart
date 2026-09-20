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

  /// 응답을 붙잡아 두고 테스트가 순서를 정한다.
  Completer<PlanResult>? held;
  bool fail = false;

  @override
  Future<String> startTrip() async => 'T1';

  @override
  Future<void> uploadTraces(String tripId, List samples) async {}

  @override
  Future<Map<String, dynamic>> endTrip(String tripId) async => const {};

  @override
  Future<PlanResult> plan(PlanRequest req) async {
    plans.add(req);
    if (fail) throw ApiException(502, '서버 없음');
    final h = held;
    return h == null ? reply : h.future;
  }
}

Position _pos(double lat, double lon, {double acc = 8}) => Position(
      latitude: lat,
      longitude: lon,
      timestamp: DateTime.now(),
      accuracy: acc,
      altitude: 0,
      altitudeAccuracy: 0,
      heading: 0,
      headingAccuracy: 0,
      speed: 0,
      speedAccuracy: 0,
    );

final _base = DateTime.now();
String _t(int m) => _base.add(Duration(minutes: m)).toIso8601String();

// 37.5 에서 북쪽으로 곧게 가는 도보. 위도 0.001도 ≈ 111m.
final _planned = encodePolyline([const LatLng(37.5, 127.0), const LatLng(37.502, 127.0)]);
// 다시 찾은 경로는 동쪽으로 돈다.
final _fresh = encodePolyline([const LatLng(37.5, 127.002), const LatLng(37.502, 127.0)]);

Leg _walk(String polyline, {required String name, double durationSec = 300}) => Leg(
      mode: 'WALK',
      durationSec: durationSec,
      distanceM: 222,
      fromName: 'Origin',
      toName: name,
      fromLat: 37.5,
      fromLon: 127.0,
      toLat: 37.502,
      toLon: 127.0,
      route: '',
      rentedBike: false,
      transitLeg: false,
      polyline: polyline,
      start: _t(0),
      end: _t(5),
    );

final _itinerary = Itinerary(
  start: _t(0),
  end: _t(5),
  durationSec: 300,
  transfers: 0,
  walkM: 222,
  legs: [_walk(_planned, name: '도착지')],
);

const _request = PlanRequest(
  origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착지', address: '', lat: 37.502, lon: 127.0),
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

  Future<void> pump(WidgetTester tester) async {
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
  }

  Future<void> push(WidgetTester tester, Position p) async {
    geo.controller.add(p);
    await _settle(tester);
  }


  // 도보 두 구간. 첫 구간 끝(37.502,127.0)에 닿으면 자동으로 둘째로 넘어간다.
  Itinerary twoWalks() => Itinerary(
        start: _t(0),
        end: _t(10),
        durationSec: 600,
        transfers: 0,
        walkM: 444,
        legs: [
          _walk(_planned, name: '중간'),
          Leg(
            mode: 'WALK',
            durationSec: 300,
            distanceM: 222,
            fromName: '중간',
            toName: '도착지',
            fromLat: 37.502,
            fromLon: 127.0,
            toLat: 37.504,
            toLon: 127.0,
            route: '',
            rentedBike: false,
            transitLeg: false,
            polyline: encodePolyline([const LatLng(37.502, 127.0), const LatLng(37.504, 127.0)]),
            start: _t(5),
            end: _t(10),
          ),
        ],
      );

  Future<void> pumpTwo(WidgetTester tester) async {
    await tester.pumpWidget(MaterialApp(
      home: GuideScreen(
          api: api,
          request: _request,
          itinerary: twoWalks(),
          speak: (t) async => spoken.add(t),
          clock: () => clock),
    ));
    await _settle(tester);
  }

  // 경로선에서 동쪽으로 약 176m 떨어진 자리. 연속 5회면 이탈로 본다.
  Position away() => _pos(37.501, 127.002);

  testWidgets('경로를 벗어나면 그 구간만 다시 찾아 갈아 끼운다', (tester) async {
    api.reply = PlanResult(itineraries: [
      Itinerary(
        start: _t(0),
        end: _t(7),
        durationSec: 420,
        transfers: 0,
        walkM: 300,
        legs: [_walk(_fresh, name: '도착지', durationSec: 420)],
      )
    ]);
    await pump(tester);
    await push(tester, _pos(37.5, 127.0));
    for (var i = 0; i < 4; i++) {
      await push(tester, away());
    }
    expect(api.plans, isEmpty); // 4회로는 아직
    await push(tester, away());
    await tester.pumpAndSettle();

    expect(api.plans.length, 1);
    final req = api.plans.single;
    expect(req.origin.name, '현재 위치');
    expect(req.destination.name, '도착지');
    expect(req.segmentModes, [SegmentMode.walk]);
    expect(ActiveGuide.instance.current!.tracker.current.polyline, _fresh);
    expect(spoken, contains('경로를 벗어나 다시 찾았습니다'));

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });

  testWidgets('다시 찾은 경로가 없으면 원래 경로로 이어 간다', (tester) async {
    await pump(tester);
    await push(tester, _pos(37.5, 127.0));
    for (var i = 0; i < 5; i++) {
      await push(tester, away());
    }
    await tester.pumpAndSettle();

    expect(api.plans.length, 1);
    expect(ActiveGuide.instance.current!.tracker.current.polyline, _planned);

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });

  testWidgets('오차가 큰 표본으로는 다시 찾지 않는다', (tester) async {
    await pump(tester);
    await push(tester, _pos(37.5, 127.0));
    for (var i = 0; i < 8; i++) {
      await push(tester, _pos(37.501, 127.002, acc: 90));
    }
    await tester.pumpAndSettle();
    expect(api.plans, isEmpty);

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });

  testWidgets('자전거 구간은 다시 찾지 않는다', (tester) async {
    final bike = Itinerary(
      start: _t(0),
      end: _t(5),
      durationSec: 300,
      transfers: 0,
      walkM: 0,
      legs: [
        Leg(
          mode: 'BICYCLE',
          durationSec: 300,
          distanceM: 222,
          fromName: 'Origin',
          toName: '도착지',
          fromLat: 37.5,
          fromLon: 127.0,
          toLat: 37.502,
          toLon: 127.0,
          route: '',
          rentedBike: true,
          transitLeg: false,
          polyline: _planned,
          start: _t(0),
          end: _t(5),
        )
      ],
    );
    await tester.pumpWidget(MaterialApp(
      home: GuideScreen(
          api: api,
          request: _request,
          itinerary: bike,
          speak: (t) async => spoken.add(t),
          clock: () => clock),
    ));
    await _settle(tester);
    await push(tester, _pos(37.5, 127.0));
    for (var i = 0; i < 8; i++) {
      await push(tester, away());
    }
    await tester.pumpAndSettle();
    expect(api.plans, isEmpty);

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });

  // 재탐색을 기다리는 동안 구간이 넘어가면 그 결과는 남의 구간 것이다.
  testWidgets('기다리는 사이 구간이 바뀌면 다시 찾은 경로를 버린다', (tester) async {
    final hold = Completer<PlanResult>();
    api.held = hold;
    final two = Itinerary(
      start: _t(0),
      end: _t(10),
      durationSec: 600,
      transfers: 0,
      walkM: 444,
      legs: [
        _walk(_planned, name: '중간'),
        Leg(
          mode: 'WALK',
          durationSec: 300,
          distanceM: 222,
          fromName: '중간',
          toName: '도착지',
          fromLat: 37.502,
          fromLon: 127.0,
          toLat: 37.504,
          toLon: 127.0,
          route: '',
          rentedBike: false,
          transitLeg: false,
          polyline: encodePolyline([const LatLng(37.502, 127.0), const LatLng(37.504, 127.0)]),
          start: _t(5),
          end: _t(10),
        ),
      ],
    );
    await tester.pumpWidget(MaterialApp(
      home: GuideScreen(
          api: api,
          request: _request,
          itinerary: two,
          speak: (t) async => spoken.add(t),
          clock: () => clock),
    ));
    await _settle(tester);
    await push(tester, _pos(37.5, 127.0));
    for (var i = 0; i < 5; i++) {
      await push(tester, away());
    }
    expect(api.plans.length, 1);

    await tester.tap(find.text('다음 구간')); // 응답 전에 구간이 넘어간다
    await _settle(tester);
    hold.complete(PlanResult(itineraries: [
      Itinerary(start: _t(0), end: _t(7), durationSec: 420, transfers: 0, walkM: 300, legs: [
        _walk(_fresh, name: '중간', durationSec: 420),
      ])
    ]));
    await tester.pumpAndSettle();

    final t = ActiveGuide.instance.current!.tracker;
    expect(t.index, 1);
    expect(t.current.toName, '도착지'); // 둘째 구간이 덮이지 않았다
    expect(t.legs.first.polyline, _planned); // 첫 구간도 그대로

    // 버렸다고 이탈 셈까지 죽으면 안 된다 — 새 구간에서 벗어나면 다시 찾아야 한다.
    api.held = null;
    clock = _base.add(const Duration(minutes: 2));
    for (var i = 0; i < 5; i++) {
      await push(tester, _pos(37.503, 127.002));
    }
    await tester.pumpAndSettle();
    expect(api.plans.length, 2);

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });

  // 제한에 걸려 버린 신호나 실패한 시도가 재탐색을 영영 멈추면 안 된다.
  testWidgets('재탐색이 실패해도 다음 이탈에 다시 찾는다', (tester) async {
    await pump(tester);
    await push(tester, _pos(37.5, 127.0));
    for (var i = 0; i < 5; i++) {
      await push(tester, away());
    }
    await tester.pumpAndSettle();
    expect(api.plans.length, 1); // 빈 결과 → 실패

    api.reply = PlanResult(itineraries: [
      Itinerary(start: _t(0), end: _t(7), durationSec: 420, transfers: 0, walkM: 300, legs: [
        _walk(_fresh, name: '도착지', durationSec: 420),
      ])
    ]);
    clock = _base.add(const Duration(minutes: 5)); // 제한 시간이 지난 뒤
    for (var i = 0; i < 5; i++) {
      await push(tester, away());
    }
    await tester.pumpAndSettle();
    expect(api.plans.length, 2);
    expect(ActiveGuide.instance.current!.tracker.current.polyline, _fresh);

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });

  // 다시 찾은 경로가 여러 구간이면 화면의 구간 수도 그만큼 늘어야 한다.
  testWidgets('여러 구간으로 다시 찾으면 화면 구간 수가 따라간다', (tester) async {
    api.reply = PlanResult(itineraries: [
      Itinerary(start: _t(0), end: _t(8), durationSec: 480, transfers: 0, walkM: 400, legs: [
        _walk(_fresh, name: '중간', durationSec: 240),
        Leg(
          mode: 'WALK',
          durationSec: 240,
          distanceM: 222,
          fromName: '중간',
          toName: '도착지',
          fromLat: 37.501,
          fromLon: 127.002,
          toLat: 37.502,
          toLon: 127.0,
          route: '',
          rentedBike: false,
          transitLeg: false,
          polyline: encodePolyline([const LatLng(37.501, 127.002), const LatLng(37.502, 127.0)]),
          start: _t(4),
          end: _t(8),
        ),
      ])
    ]);
    await pump(tester);
    await push(tester, _pos(37.5, 127.0));
    expect(find.text('안내 · 구간 1/1'), findsOneWidget);
    for (var i = 0; i < 5; i++) {
      await push(tester, away());
    }
    await tester.pumpAndSettle();
    expect(api.plans.length, 1);
    expect(find.text('안내 · 구간 1/2'), findsOneWidget);

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });

  // 1분 제한에 걸려 버린 신호가 셈을 소비한 채로 남으면 안 된다.
  testWidgets('제한에 걸린 뒤에도 시간이 지나면 다시 찾는다', (tester) async {
    api.reply = PlanResult(itineraries: [
      Itinerary(start: _t(0), end: _t(7), durationSec: 420, transfers: 0, walkM: 300, legs: [
        _walk(_fresh, name: '도착지', durationSec: 420),
      ])
    ]);
    await pump(tester);
    await push(tester, _pos(37.5, 127.0));
    for (var i = 0; i < 5; i++) {
      await push(tester, away());
    }
    await tester.pumpAndSettle();
    expect(api.plans.length, 1);

    // 다시 찾은 경로에서도 벗어나지만 아직 1분이 안 지났다 → 신호가 버려진다
    for (var i = 0; i < 5; i++) {
      await push(tester, _pos(37.501, 127.0));
    }
    await tester.pumpAndSettle();
    expect(api.plans.length, 1);

    clock = _base.add(const Duration(minutes: 2));
    for (var i = 0; i < 5; i++) {
      await push(tester, _pos(37.501, 127.0));
    }
    await tester.pumpAndSettle();
    expect(api.plans.length, 2);

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });

  testWidgets('호출이 예외로 끝나도 다음 이탈에 다시 찾는다', (tester) async {
    api.fail = true;
    await pump(tester);
    await push(tester, _pos(37.5, 127.0));
    for (var i = 0; i < 5; i++) {
      await push(tester, away());
    }
    await tester.pumpAndSettle();
    expect(api.plans.length, 1);

    api.fail = false;
    api.reply = PlanResult(itineraries: [
      Itinerary(start: _t(0), end: _t(7), durationSec: 420, transfers: 0, walkM: 300, legs: [
        _walk(_fresh, name: '도착지', durationSec: 420),
      ])
    ]);
    clock = _base.add(const Duration(minutes: 2));
    for (var i = 0; i < 5; i++) {
      await push(tester, away());
    }
    await tester.pumpAndSettle();
    expect(api.plans.length, 2);
    expect(ActiveGuide.instance.current!.tracker.current.polyline, _fresh);

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });

  // 구간이 바뀌면 이탈 셈을 지워야 한다 — 앞 구간에서 쌓인 수가 남으면 새 구간에서 바로 재탐색이 터진다.
  testWidgets('자동으로 구간이 넘어가면 이탈 셈을 지운다', (tester) async {
    await pumpTwo(tester);
    await push(tester, _pos(37.5, 127.0));
    for (var i = 0; i < 4; i++) {
      await push(tester, away()); // 첫 구간에서 4회 쌓는다
    }
    await push(tester, _pos(37.502, 127.0)); // 첫 구간 끝 → 자동 인계
    expect(ActiveGuide.instance.current!.tracker.index, 1);

    await push(tester, _pos(37.503, 127.002)); // 둘째 구간에서 첫 이탈 표본
    await tester.pumpAndSettle();
    expect(api.plans, isEmpty);

    for (var i = 0; i < 4; i++) {
      await push(tester, _pos(37.503, 127.002));
    }
    await tester.pumpAndSettle();
    expect(api.plans.length, 1); // 새로 5회를 채운 뒤에야 찾는다

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });

  testWidgets('이전 구간으로 되돌려도 이탈 셈을 지운다', (tester) async {
    await pumpTwo(tester);
    await push(tester, _pos(37.5, 127.0));
    await tester.tap(find.text('다음 구간'));
    await _settle(tester);
    for (var i = 0; i < 4; i++) {
      await push(tester, _pos(37.503, 127.002)); // 둘째 구간에서 4회
    }
    await tester.tap(find.text('이전 구간'));
    await _settle(tester);

    await push(tester, away()); // 첫 구간에서 첫 이탈 표본
    await tester.pumpAndSettle();
    expect(api.plans, isEmpty);

    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  });
}
