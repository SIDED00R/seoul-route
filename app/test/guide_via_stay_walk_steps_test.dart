import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/guide/active_guide.dart';
import 'package:seoul_route/guide/guide_session.dart';
import 'package:seoul_route/guide/guide_store.dart';
import 'package:seoul_route/models/favorite_place.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';

class _Geo extends GeolocatorPlatform {
  final controller = StreamController<Position>.broadcast();

  @override
  Future<LocationPermission> checkPermission() async => LocationPermission.whileInUse;

  @override
  Future<bool> isLocationServiceEnabled() async => true;

  @override
  Stream<Position> getPositionStream({LocationSettings? locationSettings}) => controller.stream;
}

/// 첫 모퉁이(37.502)에만 시설(우리은행)이 있는 서버.
class _Api extends ApiClient {
  _Api() : super(baseUrl: 'http://x', token: 't');

  @override
  Future<String> startTrip() async => 'T1';

  @override
  Future<void> uploadTraces(String tripId, List samples) async {}

  @override
  Future<Map<String, dynamic>> endTrip(String tripId) async => const {};

  @override
  Future<GuideLandmark?> landmark(double lat, double lon) async => (lat - 37.502).abs() < 1e-6
      ? GuideLandmark(name: '우리은행', lat: lat, lon: lon, distanceM: 8)
      : null;
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

// 09:00~09:05 경유지(코엑스, 37.501)까지 도보 → 체류 30분 → 09:35~09:40 도착지(37.503)까지 도보.
// 둘째 구간은 출발(37.501) → 첫 모퉁이 좌회전(37.502) → 도착지. 위도 0.001도 ≈ 111m.
DateTime _at(int m) => DateTime(2026, 9, 20, 9, m);

Map<String, dynamic> _walk(String to, double fromLat, double toLat, int from, int until) => {
      'mode': 'WALK',
      'duration_sec': (until - from) * 60,
      'distance_m': 111,
      'from_name': 'x',
      'to_name': to,
      'from_lat': fromLat,
      'from_lon': 127.0,
      'to_lat': toLat,
      'to_lon': 127.0,
      'start': _at(from).toIso8601String(),
      'end': _at(until).toIso8601String(),
    };

final _itinerary = Itinerary.fromJson({
  'start': _at(0).toIso8601String(),
  'end': _at(40).toIso8601String(),
  'duration_sec': 2400,
  'transfers': 0,
  'walk_distance_m': 333,
  'legs': [
    {..._walk('코엑스', 37.5, 37.501, 0, 5), 'stay_via': 1, 'stay_sec': 1800},
    {
      ..._walk('도착지', 37.501, 37.503, 35, 40),
      'steps': [
        {'dir': 'DEPART', 'distance_m': 111, 'lat': 37.501, 'lon': 127.0},
        {'dir': 'LEFT', 'distance_m': 111, 'lat': 37.502, 'lon': 127.0},
      ],
    },
  ],
});

const _request = PlanRequest(
  origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착지', address: '', lat: 37.503, lon: 127.0),
  via: [Place(name: '코엑스', address: '', lat: 37.501, lon: 127.0)],
  viaStayMin: [30],
);

// 경유지 다음 구간이 회전 단계가 있는 도보일 때의 체류 안내.
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const statusChannel = MethodChannel('seoul_route/guide_status');
  const permChannel = MethodChannel('seoul_route/notification_permission');
  late _Geo geo;
  late List<String> spoken;
  late DateTime clock;

  setUp(() {
    ActiveGuide.instance.clear();
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(statusChannel, (_) async => null);
    messenger.setMockMethodCallHandler(permChannel, (_) async => true);
    geo = _Geo();
    GeolocatorPlatform.instance = geo;
    spoken = [];
  });

  tearDown(() {
    ActiveGuide.instance.clear();
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(statusChannel, null);
    messenger.setMockMethodCallHandler(permChannel, null);
  });

  Future<void> push(Position p) async {
    geo.controller.add(p);
    await pumpEventQueue();
  }

  /// 09:10 에 경유지에 닿아 체류(09:40 까지)를 시작한 안내.
  Future<GuideSession> stayAtVia() async {
    clock = _at(10);
    final s = GuideSession(
      api: _Api(),
      request: _request,
      itinerary: _itinerary,
      speak: (t) async => spoken.add(t),
      store: const GuideStore(),
      clock: () => clock,
    );
    ActiveGuide.instance.set(s);
    await s.start();
    await push(_pos(37.501, 127.0));
    expect(s.staying, isTrue);
    return s;
  }

  test('체류를 시작할 때 체류 안내를 한 번만 읽고, 출발할 때 읽는 문장에 랜드마크가 들어간다', () async {
    final s = await stayAtVia();
    expect(spoken.where((t) => t.contains('머무른 뒤')), ['코엑스에 도착했습니다. 30분 머무른 뒤 9시 40분에 출발합니다']);

    clock = _at(40);
    await push(_pos(37.501, 127.0));
    expect(s.staying, isFalse);
    expect(spoken.last, '출발할 시간입니다. 110m 직진 후 우리은행 근처에서 좌회전');
    expect(spoken.where((t) => t.contains('머무른 뒤')).length, 1);
  });

  test('체류 중 다음 구간의 모퉁이에 다녀와도 체류가 끝나면 첫 회전부터 안내한다', () async {
    final s = await stayAtVia();
    clock = _at(20);
    await push(_pos(37.502, 127.0)); // 다음 구간의 첫 모퉁이 위
    expect(s.staying, isTrue);

    clock = _at(40);
    await push(_pos(37.501, 127.0)); // 경유지로 돌아와 체류가 끝난다
    expect(s.staying, isFalse);
    expect(s.instr.cueKey, 'L1:T1');
    expect(spoken.last, contains('좌회전'));
  });
}
