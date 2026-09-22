import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:latlong2/latlong.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/guide/active_guide.dart';
import 'package:seoul_route/models/favorite_place.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/leg_detail.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/guide_screen.dart';

import 'support/polyline_encode.dart';

class PushGeolocator extends GeolocatorPlatform {
  final controller = StreamController<Position>.broadcast();

  @override
  Future<LocationPermission> checkPermission() async =>
      LocationPermission.whileInUse;

  @override
  Future<bool> isLocationServiceEnabled() async => true;

  @override
  Stream<Position> getPositionStream({LocationSettings? locationSettings}) =>
      controller.stream;
}

class SlowApi extends ApiClient {
  SlowApi() : super(baseUrl: 'http://x', token: 't');

  Completer<void>? gate; // 테스트 본문(fake async 영역)에서 만든다 — setUp 에서 만들면 완료 콜백이 fake 큐에 안 들어간다
  int landmarkCalls = 0;
  GuideLandmark? Function(double lat, double lon)? result; // null 이면 "시설 없음"

  @override
  Future<String> startTrip() async => 'T1';

  @override
  Future<void> uploadTraces(String tripId, List samples) async {}

  @override
  Future<Map<String, dynamic>> endTrip(String tripId) async => const {};

  @override
  Future<GuideLandmark?> landmark(double lat, double lon) async {
    landmarkCalls++;
    await gate!.future;
    return result?.call(lat, lon);
  }
}

Position pos(double lat, double lon) => Position(
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
String _t(int minutes) =>
    _base.add(Duration(minutes: minutes)).toIso8601String();

final _walkLine = encodePolyline([
  const LatLng(37.5, 127.0),
  const LatLng(37.502, 127.0),
]);

final _legs = [
  Leg(
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
    polyline: _walkLine,
    start: _t(0),
    end: _t(5),
    steps: const [
      WalkStep(
        dir: 'DEPART',
        street: '테헤란로',
        distanceM: 124,
        lat: 37.5,
        lon: 127.0,
      ),
      WalkStep(dir: 'RIGHT', distanceM: 98, lat: 37.5011, lon: 127.0),
      WalkStep(
        dir: 'ENTER_STATION',
        entrance: '강남 8번 출구',
        distanceM: 0,
        lat: 37.502,
        lon: 127.0,
      ),
    ],
  ),
];

final _itinerary = Itinerary(
  start: _t(0),
  end: _t(5),
  durationSec: 300,
  transfers: 0,
  walkM: 222,
  legs: _legs,
);

const _request = PlanRequest(
  origin: Place(name: '출발지', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착지', address: '', lat: 37.502, lon: 127.0),
);

Future<void> _settle(WidgetTester tester) async {
  for (var i = 0; i < 10; i++) {
    await tester.pump();
  }
}

// 늦게 끝난 랜드마크 조회: 결과가 없으면 같은 회전 안내를 다시 읽지 않고, 실제로 시설이 붙었을 때만 한 번 더 읽는다.
// (거리 문구는 걷기만 해도 바뀌므로 그것을 "새 정보"로 오인해 중복 발화하던 결함의 회귀 테스트.)
void main() {
  const channel = MethodChannel('seoul_route/notification_permission');
  late PushGeolocator geo;
  late SlowApi api;
  late List<String> spoken;

  setUp(() {
    ActiveGuide.instance.clear();
    SharedPreferences.setMockInitialValues(<String, Object>{});
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async => true);
    geo = PushGeolocator();
    GeolocatorPlatform.instance = geo;
    api = SlowApi();
    spoken = [];
  });

  tearDown(() {
    ActiveGuide.instance.clear();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, null);
  });

  // 안내를 시작해 거리 안내가 먼저 나가고, 랜드마크 응답을 기다리는 사이 위치 표본으로 남은 거리만 바꾼 뒤 게이트를 연다.
  Future<void> startWalkAndReleaseLandmark(WidgetTester tester) async {
    api.gate = Completer<void>();
    await tester.pumpWidget(
      MaterialApp(
        home: GuideScreen(
          api: api,
          request: _request,
          itinerary: _itinerary,
          speak: (t) async => spoken.add(t),
          clock: () => _base,
        ),
      ),
    );
    await _settle(tester);
    await tester.pump(const Duration(seconds: 3)); // start() 의 2초 대기가 끝나 거리 안내로 먼저 읽는다
    await _settle(tester);
    expect(api.landmarkCalls, 2);
    expect(spoken, ['120m 직진 후 우회전']);

    geo.controller.add(pos(37.5005, 127.0)); // 같은 회전, 남은 거리만 70m 로
    await _settle(tester);
    expect(spoken.length, 1, reason: '같은 회전이라 다시 읽지 않는다');
    expect(find.text('70m 직진 후 우회전'), findsOneWidget);

    api.gate!.complete();
    for (var i = 0; i < 5; i++) {
      await Future<void>.value();
    }
    await tester.pump(const Duration(milliseconds: 1));
    await _settle(tester);
  }

  Future<void> teardownGuide(WidgetTester tester) async {
    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  }

  testWidgets('늦게 온 랜드마크 결과가 없으면 같은 회전 안내를 다시 읽지 않는다', (tester) async {
    api.result = (_, _) => null;
    await startWalkAndReleaseLandmark(tester);
    expect(find.text('70m 직진 후 우회전'), findsOneWidget);
    expect(spoken.length, 1, reason: '랜드마크가 붙지 않았으면 다시 읽지 않아야 한다');
    await teardownGuide(tester);
  });

  testWidgets('늦게 온 랜드마크가 실제로 붙으면 보강 문장을 한 번만 읽는다', (tester) async {
    api.result = (lat, lon) => (lat - 37.5011).abs() < 1e-6
        ? const GuideLandmark(name: '우리은행', lat: 37.5011, lon: 127.0, distanceM: 8)
        : null;
    await startWalkAndReleaseLandmark(tester);
    expect(spoken.length, 2, reason: '회전점에 시설이 붙었으니 한 번 더 읽는다');
    expect(spoken.last, contains('우리은행'));
    expect(find.textContaining('우리은행'), findsOneWidget);
    await teardownGuide(tester);
  });
}
