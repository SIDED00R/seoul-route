import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/guide/active_guide.dart';
import 'package:seoul_route/guide/guide_session.dart';
import 'package:seoul_route/guide/guide_store.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/models/trace_sample.dart';

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

  bool uploadFails = false;

  /// 있으면 종료 요청이 이것을 던진다.
  Object? endError;
  final List<String> ended = [];
  final List<TraceSample> uploaded = [];

  @override
  Future<String> startTrip() async => 'T1';

  @override
  Future<void> uploadTraces(String tripId, List samples) async {
    if (uploadFails) throw ApiException(503, '서버 없음');
    uploaded.addAll(samples.cast<TraceSample>());
  }

  @override
  Future<Map<String, dynamic>> endTrip(String tripId) async {
    ended.add(tripId);
    final e = endError;
    if (e != null) throw e;
    return const {};
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

// 출발(37.5) → 중간(37.501) → 도착지(37.502). 0.001도 ≈ 111m.
Map<String, dynamic> _legJson({required String to, required double fromLat, required double toLat}) => {
      'mode': 'WALK',
      'duration_sec': 300,
      'distance_m': 111,
      'from_name': '출발',
      'to_name': to,
      'from_lat': fromLat,
      'from_lon': 127.0,
      'to_lat': toLat,
      'to_lon': 127.0,
      'start': '2026-09-20T09:00:00+09:00',
      'end': '2026-09-20T09:05:00+09:00',
    };

final _itinerary = Itinerary.fromJson({
  'start': '2026-09-20T09:00:00+09:00',
  'end': '2026-09-20T09:05:00+09:00',
  'duration_sec': 300,
  'transfers': 0,
  'walk_distance_m': 222,
  'legs': [
    _legJson(to: '중간', fromLat: 37.5, toLat: 37.501),
    _legJson(to: '도착지', fromLat: 37.501, toLat: 37.502),
  ],
});

const _request = PlanRequest(
  origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착지', address: '', lat: 37.502, lon: 127.0),
);

// 종료가 실패해 이어지는 안내의 음성과 주기 업로드.
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const statusChannel = MethodChannel('seoul_route/guide_status');
  const permChannel = MethodChannel('seoul_route/notification_permission');
  late _Geo geo;
  late _Api api;
  late List<String> spoken;

  setUp(() {
    ActiveGuide.instance.clear();
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(statusChannel, (_) async => null);
    messenger.setMockMethodCallHandler(permChannel, (_) async => true);
    geo = _Geo();
    GeolocatorPlatform.instance = geo;
    api = _Api();
    spoken = [];
  });

  tearDown(() {
    ActiveGuide.instance.clear();
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(statusChannel, null);
    messenger.setMockMethodCallHandler(permChannel, null);
  });

  GuideSession newSession() {
    final s = GuideSession(
      api: api,
      request: _request,
      itinerary: _itinerary,
      speak: (t) async => spoken.add(t),
      store: const GuideStore(),
    );
    ActiveGuide.instance.set(s);
    return s;
  }

  Future<void> push(Position p) async {
    geo.controller.add(p);
    await pumpEventQueue();
  }

  for (final c in <(String, Object)>[
    ('서버 오류 응답', ApiException(503, '서버 없음')),
    ('연결 오류', Exception('연결 끊김')),
  ]) {
    test('사용자가 누른 종료가 서버 종료 실패(${c.$1})로 끝나면 다음 구간 안내를 읽는다', () async {
      final s = newSession();
      await s.start();
      api.endError = c.$2;
      expect(await s.end(), isNull);
      expect(s.status, contains('종료 실패'));
      final before = spoken.length;

      await push(_pos(37.50095, 127.0)); // 첫 구간 도착지 근처 — 둘째 구간으로
      expect(s.tracker.index, 1);
      expect(spoken.length, before + 1);
      expect(spoken.last, contains('도착지'));
    });
  }

  test('사용자가 누른 종료가 샘플 전송 실패로 끝나면 다음 구간 안내를 읽는다', () async {
    final s = newSession();
    await s.start();
    api.uploadFails = true;
    await push(_pos(37.5003, 127.0)); // 올리지 못할 샘플 하나
    expect(await s.end(), isNull);
    expect(s.status, contains('전송 실패'));
    expect(api.ended, isEmpty);
    final before = spoken.length;

    await push(_pos(37.50095, 127.0));
    expect(s.tracker.index, 1);
    expect(spoken.length, before + 1);
    expect(spoken.last, contains('도착지'));
  });

  test('도착 자동 종료가 실패하면 음성을 되돌리지 않는다', () async {
    final s = newSession();
    await s.start();
    await push(_pos(37.50095, 127.0)); // 마지막 구간으로
    api.endError = ApiException(503, '서버 없음');
    for (var i = 0; i < 3; i++) {
      await push(_pos(37.50195, 127.0)); // 도착지에서 약 6m
    }
    expect(api.ended, ['T1']);
    expect(s.ended, isFalse);
    final before = spoken.length;

    s.prevLeg();
    await pumpEventQueue();
    expect(s.tracker.index, 0);
    expect(spoken.length, before);
  });

  Future<void> settle(WidgetTester tester) async {
    for (var i = 0; i < 10; i++) {
      await tester.pump();
    }
  }

  // 주기 업로드는 세션을 만든 영역의 타이머라 testWidgets 의 가짜 시계로 흘린다.
  for (final c in <(String, Object)>[
    ('서버 오류 응답', ApiException(503, '서버 없음')),
    ('연결 오류', Exception('연결 끊김')),
  ]) {
    testWidgets('사용자가 누른 종료가 서버 종료 실패(${c.$1})로 끝난 뒤 받은 샘플을 주기 업로드로 올린다', (tester) async {
      final s = newSession();
      unawaited(s.start());
      await settle(tester);
      api.endError = c.$2;
      unawaited(s.end());
      await settle(tester);
      expect(api.ended, ['T1']);
      expect(s.ending, isFalse);

      geo.controller.add(_pos(37.5003, 127.0));
      await settle(tester);
      expect(api.uploaded, isEmpty);
      await tester.pump(const Duration(seconds: 31));
      await settle(tester);
      expect(api.uploaded.length, 1);

      ActiveGuide.instance.clear();
      await tester.pump(GuideSession.tickInterval);
    });
  }

  testWidgets('사용자가 누른 종료가 샘플 전송 실패로 끝난 뒤 남은 샘플을 주기 업로드로 올린다', (tester) async {
    final s = newSession();
    unawaited(s.start());
    await settle(tester);
    api.uploadFails = true;
    geo.controller.add(_pos(37.5003, 127.0));
    await settle(tester);
    unawaited(s.end());
    await settle(tester);
    expect(s.ending, isFalse);
    expect(s.uploader!.pending, 1);

    api.uploadFails = false;
    await tester.pump(const Duration(seconds: 31));
    await settle(tester);
    expect(api.uploaded.length, 1);

    ActiveGuide.instance.clear();
    await tester.pump(GuideSession.tickInterval);
  });
}
