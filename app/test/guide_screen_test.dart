import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:latlong2/latlong.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/guide/active_guide.dart';
import 'package:seoul_route/models/fast_exit.dart';
import 'package:seoul_route/models/favorite_place.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/leg_detail.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/guide_screen.dart';
import 'package:seoul_route/util/hhmm.dart';

import 'support/polyline_encode.dart';

/// 위치 스트림에 테스트가 직접 표본을 밀어 넣는다.
class PushGeolocator extends GeolocatorPlatform {
  final controller = StreamController<Position>.broadcast();
  final List<LocationSettings?> streamCalls = [];

  @override
  Future<LocationPermission> checkPermission() async =>
      LocationPermission.whileInUse;

  @override
  Future<bool> isLocationServiceEnabled() async => true;

  @override
  Stream<Position> getPositionStream({LocationSettings? locationSettings}) {
    streamCalls.add(locationSettings);
    return controller.stream;
  }
}

class FakeApi extends ApiClient {
  FakeApi() : super(baseUrl: 'http://x', token: 't');

  final List<List<dynamic>> uploads = [];
  final Map<double, int> landmarkCalls = {};
  double? failLandmarkOnceAt;

  @override
  Future<String> startTrip() async => 'T1';

  @override
  Future<void> uploadTraces(String tripId, List samples) async =>
      uploads.add(samples);

  @override
  Future<Map<String, dynamic>> endTrip(String tripId) async => const {};

  @override
  Future<GuideLandmark?> landmark(double lat, double lon) async {
    final calls = (landmarkCalls[lat] ?? 0) + 1;
    landmarkCalls[lat] = calls;
    if (lat == failLandmarkOnceAt && calls == 1) {
      throw ApiException(502, '일시 오류');
    }
    if (lat == failLandmarkOnceAt) {
      return GuideLandmark(name: '우리은행', lat: lat, lon: lon, distanceM: 8);
    }
    return null;
  }
}

Position pos(double lat, double lon, {double acc = 8, DateTime? ts}) =>
    Position(
      latitude: lat,
      longitude: lon,
      timestamp: ts ?? DateTime.now(),
      accuracy: acc,
      altitude: 0,
      altitudeAccuracy: 0,
      heading: 0,
      headingAccuracy: 0,
      speed: 0,
      speedAccuracy: 0,
    );

// 시각은 실행 시각 기준으로 만든다 — 고정 시각을 쓰면 StopTracker 의 시간표 판정이 벽시계에 따라 달라진다.
final _base = DateTime.now();
String _t(int minutes) =>
    _base.add(Duration(minutes: minutes)).toIso8601String();

// 도보(모퉁이 2곳·출입구) → 2호선(중간 정차 2곳) → 도보(출구). 경도 0.0001도 ≈ 8.8m, 위도 0.0001도 ≈ 11.1m.
final _walkLine = encodePolyline([
  const LatLng(37.5, 127.0),
  const LatLng(37.502, 127.0),
]);
final _subwayLine = encodePolyline([
  const LatLng(37.502, 127.0),
  const LatLng(37.506, 127.0),
]);
final _lastLine = encodePolyline([
  const LatLng(37.506, 127.0),
  const LatLng(37.508, 127.0),
]);

final _legs = [
  Leg(
    mode: 'WALK',
    durationSec: 300,
    distanceM: 222,
    fromName: 'Origin',
    toName: '강남(2호선)',
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
  Leg(
    mode: 'SUBWAY',
    durationSec: 600,
    distanceM: 445,
    fromName: '강남(2호선)',
    toName: '역삼',
    fromLat: 37.502,
    fromLon: 127.0,
    toLat: 37.506,
    toLon: 127.0,
    route: '2호선',
    rentedBike: false,
    transitLeg: true,
    polyline: _subwayLine,
    start: _t(6),
    end: _t(16),
    headsign: '성수',
    fastExit: const [
      FastExitFacility(name: '계단', doors: ['4-2', '7-1']),
    ],
    stops: const [
      TransitStop(name: '교대', lat: 37.503, lon: 127.0, offsetSec: 120),
      TransitStop(name: '서초', lat: 37.5045, lon: 127.0, offsetSec: 300),
    ],
  ),
  Leg(
    mode: 'WALK',
    durationSec: 240,
    distanceM: 222,
    fromName: '역삼',
    toName: 'Destination',
    fromLat: 37.506,
    fromLon: 127.0,
    toLat: 37.508,
    toLon: 127.0,
    route: '',
    rentedBike: false,
    transitLeg: false,
    polyline: _lastLine,
    start: _t(17),
    end: _t(21),
    steps: const [
      WalkStep(
        dir: 'EXIT_STATION',
        entrance: '역삼 1번 출구',
        distanceM: 0,
        lat: 37.506,
        lon: 127.0,
      ),
      WalkStep(
        dir: 'CONTINUE',
        street: '테헤란로',
        distanceM: 222,
        lat: 37.5061,
        lon: 127.0,
      ),
    ],
  ),
];

final _itinerary = Itinerary(
  start: _t(0),
  end: _t(21),
  durationSec: 1260,
  transfers: 0,
  walkM: 444,
  legs: _legs,
);

const _request = PlanRequest(
  origin: Place(name: '출발지', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착지', address: '', lat: 37.508, lon: 127.0),
);

Future<void> _settle(WidgetTester tester) async {
  for (var i = 0; i < 10; i++) {
    await tester.pump();
  }
}

Future<void> _push(WidgetTester tester, PushGeolocator geo, Position p) async {
  geo.controller.add(p);
  await _settle(tester);
}

void main() {
  const channel = MethodChannel('seoul_route/notification_permission');
  late PushGeolocator geo;
  late FakeApi api;
  late List<String> spoken;

  setUp(() {
    // 안내는 앱에 하나뿐이라 테스트마다 치운다 — 안 그러면 앞 테스트의 안내를 이어받는다.
    ActiveGuide.instance.clear();
    SharedPreferences.setMockInitialValues(<String, Object>{});
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async => true);
    geo = PushGeolocator();
    GeolocatorPlatform.instance = geo;
    api = FakeApi();
    spoken = [];
  });

  tearDown(() {
    ActiveGuide.instance.clear(); // 세션의 10초 타이머가 테스트 뒤에 남지 않게
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, null);
  });

  /// 시각은 픽스처를 만든 순간(_base)으로 고정한다. 실제 시계를 쓰면 LegTracker 가 구간 1 에 늦게 들어온
  /// 것으로 보고 도착 예정을 그만큼 밀어, _base 의 초에 따라 카드의 분이 한 칸 올라간다.
  Future<void> pumpGuide(WidgetTester tester, {bool voice = true}) async {
    await tester.pumpWidget(
      MaterialApp(
        home: GuideScreen(
          api: api,
          request: _request,
          itinerary: _itinerary,
          speak: voice ? (t) async => spoken.add(t) : null,
          clock: () => _base,
        ),
      ),
    );
    await _settle(tester);
  }

  /// 화면을 닫고 안내도 치운다. 안내는 화면과 별개로 살아 있어(ActiveGuide) 치우지 않으면 10초 타이머가 남는다.
  Future<void> close(WidgetTester tester) async {
    ActiveGuide.instance.clear();
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(seconds: 10));
  }

  testWidgets('위치는 2초 간격으로 요청하고 포그라운드 서비스 설정을 유지한다', (tester) async {
    await pumpGuide(tester);
    final settings = geo.streamCalls.single as AndroidSettings;
    expect(settings.intervalDuration, const Duration(seconds: 2));
    expect(settings.foregroundNotificationConfig, isNotNull);
    await close(tester);
  });

  testWidgets('첫 위치에 카드 문구와 발화가 나오고, 같은 자리면 다시 읽지 않는다', (tester) async {
    await pumpGuide(tester);
    await _push(tester, geo, pos(37.5, 127.0));
    expect(
      find.textContaining('도착 예정 ${hhmm(DateTime.parse(_t(21)))}'),
      findsOneWidget,
    );
    expect(find.text('120m 직진 후 우회전'), findsOneWidget);
    expect(find.text('다음: 탑승 · 2호선 성수 방면 · 강남(2호선)'), findsOneWidget);
    expect(spoken, ['120m 직진 후 우회전']);
    await _push(tester, geo, pos(37.5, 127.0));
    expect(spoken.length, 1); // 같은 문장은 되풀이하지 않는다
    await close(tester);
  });

  testWidgets('모퉁이를 지나면 다음 단계 문구를 읽는다', (tester) async {
    await pumpGuide(tester);
    await _push(tester, geo, pos(37.5, 127.0));
    final before = spoken.length;
    await _push(tester, geo, pos(37.50111, 127.0)); // 모퉁이에서 1m
    expect(find.textContaining('강남 8번 출구로 들어가기'), findsOneWidget);
    expect(spoken.last, contains('강남 8번 출구로 들어가기'));
    expect(
      spoken.length,
      before + 1,
      reason: '단계 변경과 랜드마크 갱신이 같은 문장을 두 번 읽으면 안 된다',
    );
    await close(tester);
  });

  testWidgets('랜드마크 일시 오류는 한 번 재시도해 첫 안내에 반영한다', (tester) async {
    api.failLandmarkOnceAt = 37.5011;
    await pumpGuide(tester);
    expect(api.landmarkCalls[37.5011], 2);
    expect(spoken.single, contains('우리은행'));
    await close(tester);
  });

  testWidgets('오차가 큰 표본은 단계를 넘기지 않는다(화면이 정확도를 넘겨야 한다)', (tester) async {
    await pumpGuide(tester);
    await _push(tester, geo, pos(37.5, 127.0));
    expect(find.text('120m 직진 후 우회전'), findsOneWidget);
    // 정확도가 좋았다면 다음 단계로 넘어갈 자리(모퉁이에서 1m). 남은 거리 표시는 이 표본으로도 갱신된다.
    await _push(tester, geo, pos(37.50111, 127.0, acc: 100));
    expect(find.textContaining('직진 후 우회전'), findsOneWidget); // 단계는 그대로
    expect(spoken.length, 1); // 단계가 바뀌지 않았으니 새로 읽지도 않는다
    await close(tester);
  });

  testWidgets('대중교통 구간은 남은 정거장을 보여 주고 한 정거장 전에 하차를 읽는다', (tester) async {
    await pumpGuide(tester);
    await _push(tester, geo, pos(37.5, 127.0));
    await tester.tap(find.text('다음 구간'));
    await _settle(tester);
    expect(find.textContaining('구간 2/3'), findsOneWidget);
    expect(spoken.last, '2호선 성수 방면을 타고 3정거장 뒤 역삼에서 내리세요');
    // 지하라 위치가 튀는 상황: 정확도가 나쁘면 시간표로 센다(구간 출발이 아직 미래라 정차 2곳이 남는다).
    await _push(tester, geo, pos(37.502, 127.0, acc: 90));
    expect(find.textContaining('정거장 뒤 역삼에서 내리기'), findsOneWidget);
    // 마지막 정차를 지나면 하차 안내
    await _push(tester, geo, pos(37.5052, 127.0));
    expect(find.text('다음 역에서 내리세요 · 역삼'), findsOneWidget);
    expect(spoken.last, '다음 역에서 내리세요. 내려서 역삼 1번 출구로 나가기');
    await close(tester);
  });

  testWidgets('다음 구간 경로선을 따라가면 구간이 자동으로 넘어간다', (tester) async {
    await pumpGuide(tester);
    await tester.tap(find.text('다음 구간'));
    await _settle(tester);
    expect(find.textContaining('구간 2/3'), findsOneWidget);
    await _push(tester, geo, pos(37.5066, 127.0)); // 마지막 구간 경로선 위 67m
    expect(find.textContaining('구간 2/3'), findsOneWidget); // 한 번으로는 안 넘어간다
    await _push(tester, geo, pos(37.5066, 127.0));
    expect(find.textContaining('구간 3/3'), findsOneWidget);
    await close(tester);
  });

  testWidgets('첫 구간이 대중교통이면 위치 전에도 정거장 수를 센다', (tester) async {
    // 출발지를 역으로 앵커링한 여정은 첫 leg 가 지하철이다. 위치 표본이 오기 전에 하차 안내가 나가면 안 된다.
    final fromStation = Itinerary(
      start: _t(0),
      end: _t(21),
      durationSec: 1260,
      transfers: 0,
      walkM: 222,
      legs: [_legs[1], _legs[2]],
    );
    await tester.pumpWidget(
      MaterialApp(
        home: GuideScreen(
          api: api,
          request: _request,
          itinerary: fromStation,
          speak: (t) async => spoken.add(t),
        ),
      ),
    );
    await _settle(tester);
    expect(find.textContaining('3정거장 뒤 역삼에서 내리기'), findsOneWidget);
    expect(spoken, ['2호선 성수 방면을 타고 3정거장 뒤 역삼에서 내리세요']);
    await close(tester);
  });

  testWidgets('지하철에서 역을 지날 때마다 다시 읽지 않는다', (tester) async {
    await pumpGuide(tester);
    await tester.tap(find.text('다음 구간'));
    await _settle(tester);
    final boarded = spoken.length;
    expect(spoken.last, '2호선 성수 방면을 타고 3정거장 뒤 역삼에서 내리세요');
    await _push(tester, geo, pos(37.5025, 127.0)); // 출발 직후
    await _push(tester, geo, pos(37.5035, 127.0)); // 교대를 지남 → 남은 2정거장
    expect(find.textContaining('2정거장 뒤 역삼에서 내리기'), findsOneWidget);
    expect(spoken.length, boarded); // 화면 숫자만 바뀌고 읽지는 않는다
    await _push(tester, geo, pos(37.5052, 127.0)); // 서초를 지남 → 다음이 하차역
    expect(spoken.length, boarded + 1);
    expect(spoken.last, startsWith('다음 역에서 내리세요'));
    await close(tester);
  });

  testWidgets('위치가 끊겨도 시간표로 구간이 넘어가고 알림창 내용이 따라온다', (tester) async {
    const status = MethodChannel('seoul_route/guide_status');
    final shown = <String>[];
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(status, (call) async {
          if (call.method == 'show') {
            shown.add((call.arguments as Map)['title'] as String);
          }
          return null;
        });
    addTearDown(
      () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(status, null),
    );
    var now = _base;
    await tester.pumpWidget(
      MaterialApp(
        home: GuideScreen(
          api: api,
          request: _request,
          itinerary: _itinerary,
          speak: (t) async => spoken.add(t),
          clock: () => now,
        ),
      ),
    );
    await _settle(tester);
    await tester.tap(find.text('다음 구간')); // 지하철 탑승(계획 +6분 출발, +16분 도착)
    await _settle(tester);
    expect(find.textContaining('구간 2/3'), findsOneWidget);
    expect(shown.last, contains('역삼에서 내리기'));

    now = _base.add(const Duration(minutes: 15));
    await tester.pump(const Duration(seconds: 10)); // 아직 도착 전
    expect(find.textContaining('구간 2/3'), findsOneWidget);

    now = _base.add(const Duration(minutes: 16, seconds: 5));
    await tester.pump(const Duration(seconds: 10)); // 위치 표본 없이 시간만 흘렀다
    await _settle(tester);
    expect(find.textContaining('구간 3/3'), findsOneWidget);
    expect(shown.last, endsWith('역삼 1번 출구로 나가서 220m 직진 하면 도착지 도착'));
    expect(shown.last, startsWith('남은 ')); // 접힌 알림에서도 남은 시간이 보인다
    await close(tester);
  });

  testWidgets('안내를 늦게 시작하면 도착 예정도 그만큼 늦다', (tester) async {
    final started = _base.add(const Duration(minutes: 3));
    await tester.pumpWidget(
      MaterialApp(
        home: GuideScreen(
          api: api,
          request: _request,
          itinerary: _itinerary,
          speak: (t) async => spoken.add(t),
          clock: () => started,
        ),
      ),
    );
    await _settle(tester);
    final eta = hhmm(
      _base.add(const Duration(minutes: 24)),
    ); // 계획 도착 +21분에 밀린 3분
    expect(find.textContaining('도착 예정 $eta'), findsOneWidget);
    await close(tester);
  });

  testWidgets('지하철 구간에서는 하차역 설비 앞 칸을 보여 준다', (tester) async {
    await pumpGuide(tester);
    expect(find.textContaining('내릴 때'), findsNothing); // 첫 구간은 도보
    await tester.tap(find.text('다음 구간'));
    await _settle(tester);
    expect(find.textContaining('내릴 때 · 계단 4-2, 7-1'), findsOneWidget);
    await close(tester);
  });

  testWidgets('음성 안내가 꺼져 있으면 읽지 않는다', (tester) async {
    SharedPreferences.setMockInitialValues(<String, Object>{
      'voice_guide': false,
    });
    await pumpGuide(tester, voice: false);
    await _push(tester, geo, pos(37.5, 127.0));
    expect(find.text('120m 직진 후 우회전'), findsOneWidget);
    expect(spoken, isEmpty);
    await close(tester);
  });

  testWidgets('서버에는 5초 간격으로 솎아 올린다', (tester) async {
    await pumpGuide(tester);
    final t0 = DateTime.utc(2026, 9, 18, 0, 0, 0);
    for (final sec in [0, 2, 4, 6, 10]) {
      await _push(
        tester,
        geo,
        pos(37.5, 127.0, ts: t0.add(Duration(seconds: sec))),
      );
    }
    expect(find.textContaining('샘플 5'), findsOneWidget); // 화면은 받은 대로 센다
    expect(
      find.textContaining('대기 2'),
      findsOneWidget,
    ); // 직전에 올린 표본과 5초 이상 벌어진 0·6초만 남는다
    await close(tester);
  });
}
