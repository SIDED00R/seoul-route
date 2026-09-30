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

class _Api extends ApiClient {
  _Api() : super(baseUrl: 'http://x', token: 't');

  final List<String> ended = [];

  /// 있으면 trip 발급 응답을 이것이 끝날 때까지 붙잡아 둔다.
  Completer<void>? tripGate;

  /// 있으면 랜드마크 응답을 이것이 끝날 때까지 붙잡아 둔다.
  Completer<void>? landmarkGate;

  @override
  Future<String> startTrip() async {
    await tripGate?.future;
    return 'T1';
  }

  @override
  Future<void> uploadTraces(String tripId, List samples) async {}

  @override
  Future<Map<String, dynamic>> endTrip(String tripId) async {
    ended.add(tripId);
    return const {};
  }

  @override
  Future<GuideLandmark?> landmark(double lat, double lon) async {
    await landmarkGate?.future;
    return null;
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

// 출발(37.5) → 모퉁이 좌회전(37.501) → 도착지(37.502). 0.001도 ≈ 111m 라 37.50195 는 도착지에서 약 6m.
final _itinerary = Itinerary.fromJson({
  'start': '2026-09-20T09:00:00+09:00',
  'end': '2026-09-20T09:05:00+09:00',
  'duration_sec': 300,
  'transfers': 0,
  'walk_distance_m': 222,
  'legs': [
    {
      'mode': 'WALK',
      'duration_sec': 300,
      'distance_m': 222,
      'from_name': '출발',
      'to_name': '도착지',
      'from_lat': 37.5,
      'from_lon': 127.0,
      'to_lat': 37.502,
      'to_lon': 127.0,
      'start': '2026-09-20T09:00:00+09:00',
      'end': '2026-09-20T09:05:00+09:00',
      'steps': [
        {'dir': 'DEPART', 'distance_m': 111, 'lat': 37.5, 'lon': 127.0},
        {'dir': 'LEFT', 'distance_m': 111, 'lat': 37.501, 'lon': 127.0},
      ],
    },
  ],
});

const _request = PlanRequest(
  origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착지', address: '', lat: 37.502, lon: 127.0),
);

const _arrivedStatus = '목적지에 도착해 안내를 끝냈습니다';

// 끝난 안내는 늦게 돌아온 시작 단계·설정 반영으로 되살아나지 않는다.
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const statusChannel = MethodChannel('seoul_route/guide_status');
  const permChannel = MethodChannel('seoul_route/notification_permission');
  const overlayChannel = MethodChannel('seoul_route/guide_overlay');
  late _Geo geo;
  late _Api api;
  late List<String> spoken;
  late List<MethodCall> overlayCalls;

  setUp(() {
    ActiveGuide.instance.clear();
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(statusChannel, (_) async => null);
    messenger.setMockMethodCallHandler(permChannel, (_) async => true);
    overlayCalls = [];
    messenger.setMockMethodCallHandler(overlayChannel, (call) async {
      overlayCalls.add(call);
      return null;
    });
    geo = _Geo();
    GeolocatorPlatform.instance = geo;
    api = _Api();
    spoken = [];
  });

  tearDown(() {
    ActiveGuide.instance.clear();
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    for (final c in [statusChannel, permChannel, overlayChannel]) {
      messenger.setMockMethodCallHandler(c, null);
    }
  });

  GuideSession newSession({bool injectSpeak = true}) {
    final s = GuideSession(
      api: api,
      request: _request,
      itinerary: _itinerary,
      speak: injectSpeak ? (t) async => spoken.add(t) : null,
      store: const GuideStore(),
    );
    ActiveGuide.instance.set(s);
    return s;
  }

  /// 도착지 반경 안 표본 셋으로 도착 자동 종료를 일으킨다.
  Future<void> arrive() async {
    for (var i = 0; i < 3; i++) {
      geo.controller.add(_pos(37.50195, 127.0));
      await pumpEventQueue();
    }
  }

  test('trip 발급을 기다리는 동안 도착해 끝난 안내는 발급된 trip 을 닫고 되살아나지 않는다', () async {
    api.tripGate = Completer<void>();
    final s = newSession();
    final starting = s.start();
    await pumpEventQueue();
    await arrive();
    expect(s.ended, isTrue);
    expect(s.status, _arrivedStatus);
    expect(api.ended, isEmpty);

    api.tripGate!.complete();
    await starting;
    await pumpEventQueue();
    expect(api.ended, ['T1']);
    expect(s.status, _arrivedStatus);
    expect(s.uploader, isNull);
    expect(spoken, isEmpty);
  });

  test('trip 발급을 기다리는 동안 사용자가 누른 종료가 진행 중이면 발급된 trip 을 닫고 안내를 시작하지 않는다', () async {
    // 알림 취소 응답을 붙잡아 종료를 진행 중(ending)에 세워 둔다.
    final cancelGate = Completer<void>();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(statusChannel, (
      call,
    ) async {
      if (call.method == 'cancel') await cancelGate.future;
      return null;
    });
    api.tripGate = Completer<void>();
    final s = newSession();
    final starting = s.start();
    await pumpEventQueue();
    final ending = s.end();
    await pumpEventQueue();
    expect(s.ending, isTrue);
    expect(s.ended, isFalse);

    api.tripGate!.complete();
    await starting;
    expect(api.ended, ['T1']);
    expect(s.uploader, isNull);
    expect(s.status, isNot('안내 중'));
    expect(spoken, isEmpty);

    cancelGate.complete();
    await ending;
    expect(s.ended, isTrue);
  });

  test('첫 랜드마크 조회를 기다리는 동안 도착해 끝난 안내는 첫 안내 문장을 읽지 않는다', () async {
    api.landmarkGate = Completer<void>();
    final s = newSession();
    final starting = s.start();
    await pumpEventQueue();
    expect(s.status, '안내 중');
    await arrive();
    expect(s.ended, isTrue);

    api.landmarkGate!.complete();
    await starting;
    await pumpEventQueue();
    expect(spoken, isEmpty);
    expect(s.status, _arrivedStatus);
  });

  test('음성 엔진 준비를 기다리는 동안 도착해 끝난 안내는 첫 안내 문장을 읽지 않는다', () async {
    const tts = MethodChannel('flutter_tts');
    final engineGate = Completer<void>();
    final calls = <String>[];
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(tts, (call) async {
      calls.add(call.method);
      if (call.method == 'getEngines') {
        await engineGate.future;
        return ['com.google.android.tts'];
      }
      return call.method == 'isLanguageAvailable' ? true : 1;
    });
    addTearDown(() => messenger.setMockMethodCallHandler(tts, null));

    final s = newSession(injectSpeak: false);
    final starting = s.start();
    await pumpEventQueue();
    expect(calls, ['getEngines']);
    await arrive();
    expect(s.ended, isTrue);

    engineGate.complete();
    await starting;
    await pumpEventQueue();
    expect(calls, isNot(contains('speak')));
  });

  test('활동 인식 권한 확인을 기다리는 동안 도착해 끝난 안내는 활동 인식을 켜지 않는다', () async {
    const actMethod = MethodChannel('flutter_activity_recognition/method');
    const actEvents = EventChannel('flutter_activity_recognition/updates');
    final permGate = Completer<void>();
    var listens = 0;
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(actMethod, (_) async {
      await permGate.future;
      return 'GRANTED';
    });
    messenger.setMockStreamHandler(actEvents, MockStreamHandler.inline(onListen: (_, _) => listens++));
    addTearDown(() {
      messenger.setMockMethodCallHandler(actMethod, null);
      messenger.setMockStreamHandler(actEvents, null);
    });

    final s = newSession();
    final starting = s.start();
    await pumpEventQueue();
    expect(spoken, isNotEmpty);
    await arrive();
    expect(s.ended, isTrue);

    permGate.complete();
    await starting;
    await pumpEventQueue();
    expect(s.activityOn, isFalse);
    expect(listens, 0);
  });

  test('끝난 안내에 미니 지도 설정을 다시 반영해도 네이티브에 켜라고 보내지 않는다', () async {
    SharedPreferences.setMockInitialValues(<String, Object>{'overlay_guide': true});
    final s = newSession();
    await s.start();
    expect(overlayCalls.where((c) => c.method == 'setEnabled').map((c) => (c.arguments as Map)['enabled']), [true]);
    await s.end();
    await pumpEventQueue();
    expect(s.ended, isTrue);
    overlayCalls.clear();

    await s.refreshOverlaySetting();
    await pumpEventQueue();
    expect(overlayCalls, isEmpty);
  });
}
