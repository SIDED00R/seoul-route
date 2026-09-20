import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/guide/active_guide.dart';
import 'package:seoul_route/guide/guide_restore.dart';
import 'package:seoul_route/guide/guide_session.dart';
import 'package:seoul_route/guide/guide_store.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/home_screen.dart';
import 'package:seoul_route/settings/settings_store.dart';

class _Geo extends GeolocatorPlatform {
  final controller = StreamController<Position>.broadcast();
  LocationPermission permission = LocationPermission.whileInUse;
  bool serviceEnabled = true;

  /// 권한을 확인하는 동안(플랫폼 왕복) 할 일. 복원 중 새 안내가 끼어드는 상황을 흉내 낸다.
  void Function()? onCheckPermission;

  @override
  Future<LocationPermission> checkPermission() async {
    onCheckPermission?.call();
    return permission;
  }

  @override
  Future<bool> isLocationServiceEnabled() async => serviceEnabled;

  @override
  Stream<Position> getPositionStream({LocationSettings? locationSettings}) => controller.stream;
}

class _Api extends ApiClient {
  _Api() : super(baseUrl: 'http://x', token: 't');

  int startCalls = 0;
  final List<String> ended = [];
  bool endFails = false;

  @override
  Future<String> startTrip() async {
    startCalls++;
    return 'T$startCalls';
  }

  @override
  Future<void> uploadTraces(String tripId, List samples) async {}

  @override
  Future<Map<String, dynamic>> endTrip(String tripId) async {
    ended.add(tripId);
    if (endFails) throw ApiException(503, '서버 없음');
    return const {'samples': 3};
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

// 도착지는 (37.502, 127.0). 0.001도 ≈ 111m 라 37.5015 는 약 55m 밖, 37.50195 는 약 6m 안이다.
Map<String, dynamic> _legJson({required String to, required double toLat}) => {
      'mode': 'WALK',
      'duration_sec': 300,
      'distance_m': 222,
      'from_name': '출발',
      'to_name': to,
      'from_lat': 37.5,
      'from_lon': 127.0,
      'to_lat': toLat,
      'to_lon': 127.0,
      'start': '2026-09-20T09:00:00+09:00',
      'end': '2026-09-20T09:05:00+09:00',
    };

Map<String, dynamic> _itineraryJson(List<Map<String, dynamic>> legs) => {
      'start': '2026-09-20T09:00:00+09:00',
      'end': '2026-09-20T09:05:00+09:00',
      'duration_sec': 300,
      'transfers': 0,
      'walk_distance_m': 222,
      'legs': legs,
    };

final _oneLeg = [_legJson(to: '도착지', toLat: 37.502)];
final _twoLegs = [
  _legJson(to: '중간', toLat: 37.501),
  {..._legJson(to: '도착지', toLat: 37.502), 'from_lat': 37.501},
];

const _request = PlanRequest(
  origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착지', address: '', lat: 37.502, lon: 127.0),
);

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const store = GuideStore();
  const statusChannel = MethodChannel('seoul_route/guide_status');
  const permChannel = MethodChannel('seoul_route/notification_permission');
  const taskChannel = MethodChannel('seoul_route/app_task');
  late _Geo geo;
  late _Api api;
  late List<String> spoken;
  late List<String> taskCalls;

  setUp(() {
    ActiveGuide.instance.clear();
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(statusChannel, (_) async => null);
    messenger.setMockMethodCallHandler(permChannel, (_) async => true);
    taskCalls = [];
    messenger.setMockMethodCallHandler(taskChannel, (call) async {
      taskCalls.add(call.method);
      return true;
    });
    geo = _Geo();
    GeolocatorPlatform.instance = geo;
    api = _Api();
    spoken = [];
  });

  tearDown(() {
    ActiveGuide.instance.clear();
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    for (final c in [statusChannel, permChannel, taskChannel]) {
      messenger.setMockMethodCallHandler(c, null);
    }
  });

  Future<GuideSession> startSession(List<Map<String, dynamic>> legs) async {
    final s = GuideSession(
      api: api,
      request: _request,
      itinerary: Itinerary.fromJson(_itineraryJson(legs)),
      speak: (t) async => spoken.add(t),
      store: store,
    );
    ActiveGuide.instance.set(s);
    await s.start();
    return s;
  }

  Future<void> push(Position p) async {
    geo.controller.add(p);
    await pumpEventQueue();
  }

  test('마지막 구간에서 목적지에 닿으면 사용자가 누르지 않아도 끝난다', () async {
    final s = await startSession(_oneLeg);
    for (var i = 0; i < 3; i++) {
      await push(_pos(37.50195, 127.0));
    }
    expect(api.ended, ['T1']);
    expect(s.ended, isTrue);
    expect(spoken, contains('목적지에 도착했습니다'));
  });

  test('반경 안 표본이 두 번뿐이면 아직 끝내지 않는다', () async {
    final s = await startSession(_oneLeg);
    for (var i = 0; i < 2; i++) {
      await push(_pos(37.50195, 127.0));
    }
    expect(api.ended, isEmpty);
    expect(s.ended, isFalse);
  });

  test('반경 밖에서는 아무리 받아도 끝내지 않는다', () async {
    final s = await startSession(_oneLeg);
    for (var i = 0; i < 6; i++) {
      await push(_pos(37.5015, 127.0)); // 약 55m — 반경 40m 밖
    }
    expect(api.ended, isEmpty);
    expect(s.ended, isFalse);
  });

  test('마지막 구간이 아니면 그 구간 도착지에 닿아도 끝내지 않는다(다음 구간으로 넘어간다)', () async {
    final s = await startSession(_twoLegs);
    for (var i = 0; i < 3; i++) {
      await push(_pos(37.50095, 127.0)); // 첫 구간 도착지(37.501) 근처
    }
    expect(api.ended, isEmpty);
    expect(s.ended, isFalse);
    expect(s.tracker.index, 1);
  });

  // end() 는 서버에 닫기를 요청하기 전에 위치 스트림을 끊는다. 실패해도 표본이 더 들어오지 않으므로
  // 자동 종료가 되풀이되지 않고, 사유를 보고 사용자가 종료를 다시 누른다.
  test('자동 종료가 실패하면 사유를 남기고 다시 시도하지 않는다', () async {
    api.endFails = true;
    final s = await startSession(_oneLeg);
    for (var i = 0; i < 8; i++) {
      await push(_pos(37.50195, 127.0));
    }
    expect(api.ended.length, 1);
    expect(s.ended, isFalse);
    expect(s.status, contains('종료 실패'));
  });

  test('구간을 손으로 옮기면 도착 셈을 다시 센다', () async {
    final s = await startSession(_twoLegs);
    await push(_pos(37.50095, 127.0)); // 마지막 구간으로
    for (var i = 0; i < 2; i++) {
      await push(_pos(37.50195, 127.0));
    }
    s.prevLeg();
    s.nextLeg();
    for (var i = 0; i < 2; i++) {
      await push(_pos(37.50195, 127.0));
    }
    expect(api.ended, isEmpty, reason: '셈이 지워지지 않았으면 벌써 끝났다');
    await push(_pos(37.50195, 127.0));
    expect(api.ended, ['T1']);
  });

  test('안내를 시작하면 디스크에 남고, 끝나면 지운다', () async {
    final s = await startSession(_twoLegs);
    var snap = await store.load();
    expect(snap, isNotNull);
    expect(snap!.tripId, 'T1');
    expect(snap.legIndex, 0);

    // 구간이 넘어가면 그 자리가 남는다.
    await push(_pos(37.50095, 127.0));
    snap = await store.load();
    expect(snap!.legIndex, 1);

    await s.end();
    expect(await store.load(), isNull);
  });

  test('남은 안내를 이어받으면 trip 을 새로 내지 않고 저장된 구간에서 시작한다', () async {
    final first = await startSession(_twoLegs);
    await push(_pos(37.50095, 127.0)); // 둘째 구간으로
    expect(first.tracker.index, 1);
    // 프로세스가 죽은 셈: 세션만 버리고 디스크 값은 그대로 둔다.
    final saved = await store.load();
    ActiveGuide.instance.session.value = null;

    api.startCalls = 0;
    final resumed = await restoreGuide(api, store: store);
    expect(resumed, isTrue);
    final s = ActiveGuide.instance.current!;
    expect(api.startCalls, 0, reason: 'trip 을 새로 내면 앞 trip 이 열린 채 남는다');
    expect(s.tracker.index, 1);
    expect(s.tracker.legs.length, 2);
    expect(s.startedAt, saved!.startedAt);
    await s.end();
    expect(api.ended, ['T1'], reason: '이어받은 trip 을 닫아야 한다');
  });

  test('남은 안내가 없으면 이어받지 않는다', () async {
    expect(await restoreGuide(api, store: store), isFalse);
    expect(ActiveGuide.instance.current, isNull);
  });

  // 위치를 못 쓰면 세션을 만들지 않는다. 만들면 스트림도 업로더도 없는 안내가 활성으로 남고, 그걸 치우는 순간
  // dispose 가 디스크에 남은 것까지 지워 되살릴 길이 없어진다.
  for (final c in [
    ('위치 권한이 없으면', () => geo.permission = LocationPermission.denied),
    ('기기 위치가 꺼져 있으면', () => geo.serviceEnabled = false),
  ]) {
    test('${c.$1} 이어받지 않고 디스크 값은 남긴다', () async {
      final first = await startSession(_twoLegs);
      first.dispose();
      ActiveGuide.instance.session.value = null;
      expect(await store.load(), isNull, reason: 'dispose 는 남긴 안내를 지운다');

      // 프로세스가 죽은 셈으로 다시 남긴다.
      await startSession(_twoLegs);
      ActiveGuide.instance.session.value = null;
      c.$2();

      expect(await restoreGuide(api, store: store), isFalse);
      expect(ActiveGuide.instance.current, isNull);
      expect(await store.load(), isNotNull, reason: '권한이 갖춰진 다음 실행에서 되살아나야 한다');
    });
  }

  test('권한을 확인하는 사이 새 안내가 걸리면 덮어쓰지 않는다', () async {
    await startSession(_twoLegs);
    ActiveGuide.instance.session.value = null;
    final other = GuideSession(
      api: api,
      request: _request,
      itinerary: Itinerary.fromJson(_itineraryJson(_oneLeg)),
      store: store,
    );
    geo.onCheckPermission = () => ActiveGuide.instance.session.value = other;

    expect(await restoreGuide(api, store: store), isFalse);
    expect(identical(ActiveGuide.instance.current, other), isTrue);
  });

  testWidgets('안내 중 홈에서 뒤로가기는 앱을 끝내지 않고 뒤로 보낸다', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: HomeScreen(
        settings: const Settings(baseUrl: '', token: ''),
        restore: (_) async => false,
      ),
    ));
    await tester.pumpAndSettle();

    // 안내 중이 아니면 평소대로 뒤로가기가 먹는다(앱 종료).
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(taskCalls, isEmpty);

    final s = GuideSession(
      api: api,
      request: _request,
      itinerary: Itinerary.fromJson(_itineraryJson(_oneLeg)),
      speak: (t) async => spoken.add(t),
      store: store,
    );
    ActiveGuide.instance.set(s);
    await tester.pumpAndSettle();

    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(taskCalls, ['moveToBack']);
    expect(find.byType(HomeScreen), findsOneWidget, reason: '화면이 그대로 남아야 한다');
  });
}
