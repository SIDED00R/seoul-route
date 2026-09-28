import 'dart:async';
import 'dart:convert';

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
import 'package:seoul_route/models/trace_sample.dart';
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
  int planCalls = 0;
  final List<TraceSample> uploaded = [];

  /// 종료 요청을 받는 순간에 할 일. 그때의 상태(위치 스트림이 살아 있는지)를 잰다.
  void Function()? onEnd;

  /// 있으면 종료 응답을 이것이 끝날 때까지 붙잡아 둔다.
  Completer<void>? endGate;

  /// 있으면 재탐색 응답으로 이것의 결과를 준다(없으면 실패).
  Completer<PlanResult>? planGate;

  @override
  Future<String> startTrip() async {
    startCalls++;
    return 'T$startCalls';
  }

  @override
  Future<void> uploadTraces(String tripId, List samples) async => uploaded.addAll(samples.cast<TraceSample>());

  @override
  Future<Map<String, dynamic>> endTrip(String tripId) async {
    ended.add(tripId);
    onEnd?.call();
    await endGate?.future;
    if (endFails) throw ApiException(503, '서버 없음');
    return const {'samples': 3};
  }

  @override
  Future<PlanResult> plan(PlanRequest req) async {
    planCalls++;
    final gate = planGate;
    if (gate != null) return gate.future;
    throw ApiException(503, '서버 없음');
  }
}

Position _pos(double lat, double lon, {double acc = 8, DateTime? ts}) => Position(
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
  late List<String> statusCalls;

  setUp(() {
    ActiveGuide.instance.clear();
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    statusCalls = [];
    messenger.setMockMethodCallHandler(statusChannel, (call) async {
      statusCalls.add(call.method);
      return null;
    });
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

  // 위치 스트림이 포그라운드 서비스라, 끊긴 뒤에는 백그라운드 앱이 네트워크를 잃어 종료 요청이 나가지 못한다.
  test('종료 요청을 보내는 동안 위치 스트림이 살아 있고, 성공한 뒤에 끊는다', () async {
    final listening = <bool>[];
    api.onEnd = () => listening.add(geo.controller.hasListener);
    final s = await startSession(_oneLeg);
    for (var i = 0; i < 3; i++) {
      await push(_pos(37.50195, 127.0));
    }
    expect(listening, [true]);
    expect(s.ended, isTrue);
    expect(geo.controller.hasListener, isFalse);
  });

  test('자동 종료가 실패해도 위치 스트림을 살려 두고, 표본으로는 다시 보내지 않는다', () async {
    api.endFails = true;
    final s = await startSession(_oneLeg);
    for (var i = 0; i < 3; i++) {
      await push(_pos(37.50195, 127.0));
    }
    final samples = s.samples;
    for (var i = 0; i < 5; i++) {
      await push(_pos(37.50195, 127.0));
    }
    expect(api.ended.length, 1, reason: '다시 보내는 것은 10초 점검이 맡는다');
    expect(s.ended, isFalse);
    expect(s.status, contains('종료 실패'));
    expect(geo.controller.hasListener, isTrue);
    expect(s.samples, samples + 5);
  });

  /// 폰 음성 엔진(flutter_tts 채널)을 흉내 내고, speak(문장)·stop 호출을 차례로 적는다. speak 를 넘기지 않은 세션은
  /// 실제 TtsSpeaker 로 이 채널을 부른다.
  List<String> mockTts() {
    const tts = MethodChannel('flutter_tts');
    final calls = <String>[];
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(tts, (call) async {
      // 안드로이드는 {text, focus} 맵, 그 밖의 플랫폼(테스트 호스트)은 문장만 보낸다.
      final args = call.arguments;
      calls.add(call.method == 'speak' ? 'speak:${args is Map ? args['text'] : args}' : call.method);
      return switch (call.method) {
        'getEngines' => ['com.google.android.tts'],
        'isLanguageAvailable' => true,
        _ => 1,
      };
    });
    addTearDown(() => messenger.setMockMethodCallHandler(tts, null));
    return calls;
  }

  Future<GuideSession> startSpeaking() async {
    final s = GuideSession(
      api: api,
      request: _request,
      itinerary: Itinerary.fromJson(_itineraryJson(_oneLeg)),
      store: store,
    );
    ActiveGuide.instance.set(s);
    await s.start();
    return s;
  }

  test('스스로 끝낼 때 도착 안내 문장을 끊지 않는다', () async {
    final calls = mockTts();
    final s = await startSpeaking();
    for (var i = 0; i < 3; i++) {
      await push(_pos(37.50195, 127.0));
    }
    expect(s.ended, isTrue);
    final arrived = calls.indexOf('speak:목적지에 도착했습니다');
    expect(arrived, greaterThanOrEqualTo(0));
    expect(calls.sublist(arrived), isNot(contains('stop')));
  });

  test('사용자가 종료하면 서버 응답을 기다리지 않고 읽던 문장을 멈춘다', () async {
    final calls = mockTts();
    api.endGate = Completer<void>();
    final s = await startSpeaking();
    final ending = s.end();
    await pumpEventQueue();
    expect(calls, contains('stop'));
    api.endGate!.complete();
    await ending;
  });

  test('도착해 끝낸 뒤 재탐색이 실패로 돌아와도 끝났다는 문구를 덮지 않는다', () async {
    api.planGate = Completer<PlanResult>();
    final s = await startSession(_oneLeg);
    for (var i = 0; i < 5; i++) {
      await push(_pos(37.50195, 127.0015)); // 경로선에서 약 130m — 다섯째에 재탐색 요청
    }
    for (var i = 0; i < 3; i++) {
      await push(_pos(37.50195, 127.0));
    }
    expect(s.ended, isTrue);
    api.planGate!.completeError(ApiException(503, '서버 없음'));
    await pumpEventQueue();
    expect(s.status, '목적지에 도착해 안내를 끝냈습니다');
  });

  test('도착 뒤 종료를 못 보낸 동안 들어온 표본은 서버에 올리지 않는다', () async {
    api.endFails = true;
    final s = await startSession(_oneLeg);
    final t0 = DateTime(2026, 9, 20, 9, 4);
    for (var i = 0; i < 5; i++) {
      await push(_pos(37.50195, 127.0, ts: t0.add(Duration(seconds: 5 * i)))); // 셋째가 도착 확정
    }
    api.endFails = false;
    await s.end();
    expect(s.ended, isTrue);
    expect(api.uploaded.map((e) => e.ts), [
      for (var i = 0; i < 3; i++) t0.add(Duration(seconds: 5 * i)),
    ]);
  });

  test('재탐색 응답을 기다리는 사이 도착했으면 늦게 온 경로를 끼우지 않는다', () async {
    api.planGate = Completer<PlanResult>();
    api.endFails = true;
    final s = await startSession(_oneLeg);
    for (var i = 0; i < 5; i++) {
      await push(_pos(37.50195, 127.0015)); // 경로선에서 약 130m — 다섯째에 재탐색 요청
    }
    expect(api.planCalls, 1);
    for (var i = 0; i < 3; i++) {
      await push(_pos(37.50195, 127.0));
    }
    expect(api.ended, ['T1']);
    final leg = s.tracker.current;
    api.planGate!.complete(PlanResult(itineraries: [
      Itinerary.fromJson(_itineraryJson([_legJson(to: '도착지', toLat: 37.502)])),
    ]));
    await pumpEventQueue();
    expect(identical(s.tracker.current, leg), isTrue, reason: '갈아 끼우면 도착 상태가 지워져 종료를 다시 보내지 않는다');
    expect(s.status, contains('종료 실패'));
  });

  test('종료 응답을 기다리는 사이 안내를 버리고 새로 시작하면, 늦은 응답이 새 안내를 건드리지 않는다', () async {
    api.endGate = Completer<void>();
    final old = await startSession(_oneLeg);
    final ending = old.end();
    await pumpEventQueue();
    final fresh = await startSession(_oneLeg); // 앞 안내를 놓는다(dispose)
    await pumpEventQueue();
    statusCalls.clear();
    api.endGate!.complete();
    await ending;
    await pumpEventQueue();
    expect(statusCalls, isNot(contains('cancel')));
    expect((await store.load())?.tripId, 'T2');
    expect(fresh.ended, isFalse);
  });

  test('종료를 겹쳐 불러도 서버에는 한 번만 보낸다', () async {
    final s = await startSession(_oneLeg);
    await Future.wait([s.end(), s.end()]);
    expect(api.ended, ['T1']);
  });

  test('도착 뒤 종료를 못 보낸 동안에는 경로를 벗어나도 다시 찾지 않는다', () async {
    api.endFails = true;
    final s = await startSession(_oneLeg);
    for (var i = 0; i < 3; i++) {
      await push(_pos(37.50195, 127.0));
    }
    for (var i = 0; i < 6; i++) {
      await push(_pos(37.50195, 127.0015)); // 경로선에서 약 130m
    }
    expect(api.planCalls, 0);
    expect(s.ended, isFalse);
  });

  // 10초 점검은 세션을 만든 영역의 타이머라 testWidgets 의 가짜 시계로 흘린다.
  Future<void> settle(WidgetTester tester) async {
    for (var i = 0; i < 10; i++) {
      await tester.pump();
    }
  }

  Future<GuideSession> startTicking(
    WidgetTester tester,
    List<Map<String, dynamic>> legs,
    DateTime Function() clock,
  ) async {
    final s = GuideSession(
      api: api,
      request: _request,
      itinerary: Itinerary.fromJson(_itineraryJson(legs)),
      speak: (t) async => spoken.add(t),
      store: store,
      clock: clock,
    );
    ActiveGuide.instance.set(s);
    unawaited(s.start());
    await settle(tester);
    return s;
  }

  Future<void> arrive(WidgetTester tester) async {
    for (var i = 0; i < 3; i++) {
      geo.controller.add(_pos(37.50195, 127.0));
      await settle(tester);
    }
  }

  // 스트림 구독 해제(cancel)의 Future 는 루트 zone 이라 가짜 시계 pump 로는 흐르지 않는다 — runAsync 로 한 번 흘린다.
  Future<void> tick(WidgetTester tester) async {
    await tester.pump(GuideSession.tickInterval);
    await settle(tester);
    await tester.runAsync(() => Future<void>.delayed(Duration.zero));
    await settle(tester);
  }

  testWidgets('자동 종료가 실패하면 10초 점검마다 다시 보내 끝낸다', (tester) async {
    var now = DateTime(2026, 9, 20, 9, 5);
    api.endFails = true;
    final s = await startTicking(tester, _oneLeg, () => now);
    await arrive(tester);
    expect(api.ended, ['T1']);
    expect(s.ended, isFalse);

    now = now.add(GuideSession.tickInterval);
    await tick(tester);
    expect(api.ended, ['T1', 'T1'], reason: '아직 실패 중이면 또 보낸다');
    expect(statusCalls, isNot(contains('cancel')), reason: '다시 보내는 동안 알림창은 그대로 둔다');

    api.endFails = false;
    now = now.add(GuideSession.tickInterval);
    await tick(tester);
    expect(api.ended, ['T1', 'T1', 'T1']);
    expect(s.ended, isTrue);
    expect(statusCalls.last, 'cancel');
    expect(s.status, '목적지에 도착해 안내를 끝냈습니다');
    expect(geo.controller.hasListener, isFalse);
    expect(spoken.where((t) => t == '목적지에 도착했습니다').length, 1);

    now = now.add(GuideSession.tickInterval);
    await tick(tester);
    expect(api.ended.length, 3, reason: '끝난 뒤에는 보내지 않는다');
    ActiveGuide.instance.clear();
    await tester.pump(GuideSession.tickInterval);
  });

  testWidgets('도착 뒤 기한이 지나도록 못 보내면 위치를 끊고 사용자에게 넘긴다', (tester) async {
    var now = DateTime(2026, 9, 20, 9, 5);
    api.endFails = true;
    final s = await startTicking(tester, _oneLeg, () => now);
    await arrive(tester);
    now = now.add(GuideSession.arrivalRetryFor - const Duration(seconds: 1));
    await tick(tester);
    expect(geo.controller.hasListener, isTrue, reason: '기한 안에서는 위치를 살려 둔다');

    now = now.add(const Duration(seconds: 1));
    await tick(tester);
    final sent = api.ended.length;
    expect(geo.controller.hasListener, isFalse);
    expect(s.ended, isFalse);
    expect(s.status, contains('다시 종료를 누르세요'));

    now = now.add(GuideSession.tickInterval);
    await tick(tester);
    expect(api.ended.length, sent, reason: '넘긴 뒤에는 저절로 보내지 않는다');

    api.endFails = false;
    expect(await tester.runAsync(s.end), isNotNull, reason: '사용자가 누르면 끝난다');
    expect(s.ended, isTrue);
    ActiveGuide.instance.clear();
    await tester.pump(GuideSession.tickInterval);
  });

  testWidgets('도착 뒤 구간을 손으로 되돌리면 종료를 다시 보내지 않는다', (tester) async {
    var now = DateTime(2026, 9, 20, 9, 5);
    api.endFails = true;
    final s = await startTicking(tester, _twoLegs, () => now);
    geo.controller.add(_pos(37.50095, 127.0)); // 마지막 구간으로
    await settle(tester);
    await arrive(tester);
    expect(api.ended, ['T1']);

    s.prevLeg();
    now = now.add(GuideSession.tickInterval);
    await tick(tester);
    expect(api.ended, ['T1']);
    expect(s.tracker.index, 0);
    ActiveGuide.instance.clear();
    await tester.pump(GuideSession.tickInterval);
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

  group('경유지 체류(이슈 #130)', () {
    // 09:00~09:05 경유지(코엑스, 37.501)까지 도보 → 체류 30분 → 09:35~09:40 도착지(37.502)까지 도보.
    // 시각은 지역 시간으로 만들어 기기 시간대와 무관하게 문구를 비교한다.
    DateTime at(int m) => DateTime(2026, 9, 20, 9, m);
    Map<String, dynamic> walk(String to, double fromLat, double toLat, int from, int until, {bool stay = false}) => {
          'mode': 'WALK',
          'duration_sec': (until - from) * 60,
          'distance_m': 111,
          'from_name': 'x',
          'to_name': to,
          'from_lat': fromLat,
          'from_lon': 127.0,
          'to_lat': toLat,
          'to_lon': 127.0,
          'start': at(from).toIso8601String(),
          'end': at(until).toIso8601String(),
          if (stay) 'stay_via': 1,
          if (stay) 'stay_sec': 1800,
        };
    final itinerary = Itinerary.fromJson({
      'start': at(0).toIso8601String(),
      'end': at(40).toIso8601String(),
      'duration_sec': 2400,
      'transfers': 0,
      'walk_distance_m': 222,
      'legs': [walk('코엑스', 37.5, 37.501, 0, 5, stay: true), walk('도착지', 37.501, 37.502, 35, 40)],
    });
    const viaRequest = PlanRequest(
      origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
      destination: Place(name: '도착지', address: '', lat: 37.502, lon: 127.0),
      via: [Place(name: '코엑스', address: '', lat: 37.501, lon: 127.0)],
      viaStayMin: [30],
    );
    late DateTime clock;

    Future<GuideSession> stayAtVia() async {
      clock = at(10); // 계획(09:05)보다 5분 늦게 경유지에 닿는다
      final s = GuideSession(
        api: api,
        request: viaRequest,
        itinerary: itinerary,
        speak: (t) async => spoken.add(t),
        store: store,
        clock: () => clock,
      );
      ActiveGuide.instance.set(s);
      await s.start();
      await push(_pos(37.501, 127.0));
      return s;
    }

    test('경유지에 닿으면 체류 문구·음성으로 바뀌고, 도착 예정은 늦게 닿은 만큼 밀린다', () async {
      final s = await stayAtVia();
      expect(s.staying, isTrue);
      expect(s.tracker.index, 1);
      expect(s.stayUntil, at(40));
      expect(s.instr.now, '코엑스에서 머무는 중 · 09:40 출발 · 30분 남음');
      expect(spoken.last, '코엑스에 도착했습니다. 30분 머무른 뒤 9시 40분에 출발합니다');
      expect(s.eta, at(45)); // 계획 09:40 도착 + 경유지를 5분 늦게 떠남
    });

    test('머무는 동안은 멀리 둘러봐도 재탐색하거나 구간을 넘기지 않고, 시각이 되면 다음 구간 안내를 읽는다', () async {
      final s = await stayAtVia();
      for (var i = 0; i < 6; i++) {
        clock = at(11 + i);
        await push(_pos(37.5035, 127.0)); // 도보 경로선에서 약 170m, 도착지 반경 밖
      }
      expect(api.planCalls, 0);
      expect(s.staying, isTrue);
      expect(s.tracker.index, 1);
      clock = at(40);
      await push(_pos(37.501, 127.0));
      expect(s.staying, isFalse);
      expect(spoken.last, startsWith('출발할 시간입니다. '));
      expect(s.instr.now, isNot(contains('머무는 중')));
    });

    test('"지금 출발"은 체류를 바로 끝내고 도착 예정을 지금 출발 기준으로 다시 잰다', () async {
      final s = await stayAtVia();
      clock = at(20);
      s.endStay();
      expect(s.staying, isFalse);
      expect(spoken.last, startsWith('출발할 시간입니다'));
      expect(s.eta, at(40)); // 09:20 출발은 계획(09:35)보다 이르므로 밀린 시간 0
    });

    test('체류 중에 저장한 안내를 이어받으면 체류도 이어진다', () async {
      await stayAtVia();
      final snap = await store.load(now: at(10));
      expect(snap?.stayUntil, at(40));
      final resumed = GuideSession.resume(snap!, api: api);
      expect(resumed.staying, isTrue);
      expect(resumed.instr.now, startsWith('코엑스에서 머무는 중'));
      resumed.dispose();
    });

    test('경유지로 가다 재탐색해도 새 경로 끝에서 체류가 시작되고 저장본에도 남는다', () async {
      api.planGate = Completer<PlanResult>();
      clock = at(3);
      final s = GuideSession(
        api: api,
        request: viaRequest,
        itinerary: itinerary,
        speak: (t) async => spoken.add(t),
        store: store,
        clock: () => clock,
      );
      ActiveGuide.instance.set(s);
      await s.start();
      for (var i = 0; i < 5; i++) {
        await push(_pos(37.5005, 127.0015)); // 첫 도보 경로선에서 약 130m — 다섯째에 재탐색 요청
      }
      expect(api.planCalls, 1);
      api.planGate!.complete(PlanResult(itineraries: [
        Itinerary.fromJson({
          'start': at(3).toIso8601String(),
          'end': at(6).toIso8601String(),
          'duration_sec': 180,
          'transfers': 0,
          'walk_distance_m': 60,
          'legs': [walk('코엑스', 37.5005, 37.501, 3, 6)], // 도보 재탐색 응답에는 체류 표식이 없다
        }),
      ]));
      await pumpEventQueue();
      expect(s.tracker.current.stayVia, 1);
      clock = at(10);
      await push(_pos(37.501, 127.0));
      expect(s.staying, isTrue);
      expect((await store.load(now: at(10)))!.legs.first.stayVia, 1);
    });

    test('체류 중에는 도착지 가까이 있어도 목적지 도착으로 끝내지 않는다', () async {
      final nearEnd = Itinerary.fromJson({
        'start': at(0).toIso8601String(),
        'end': at(36).toIso8601String(),
        'duration_sec': 2160,
        'transfers': 0,
        'walk_distance_m': 130,
        'legs': [walk('코엑스', 37.5, 37.501, 0, 5, stay: true), walk('도착지', 37.501, 37.5012, 35, 36)],
      });
      clock = at(10);
      final s = GuideSession(
        api: api,
        request: viaRequest,
        itinerary: nearEnd,
        speak: (t) async => spoken.add(t),
        store: store,
        clock: () => clock,
      );
      ActiveGuide.instance.set(s);
      await s.start();
      await push(_pos(37.501, 127.0));
      expect(s.staying, isTrue);
      for (var i = 0; i < 3; i++) {
        await push(_pos(37.5012, 127.0)); // 경유지에서 약 22m 인 도착지
      }
      expect(s.ended, isFalse);
      expect(api.ended, isEmpty);
    });

    test('체류가 이미 끝난 저장본을 되살리면 시작할 때 체류를 끝내고 지난 체류 문구를 읽지 않는다', () async {
      await stayAtVia();
      final snap = (await store.load(now: at(10)))!;
      // resume 은 기기 시계를 쓰므로 체류 끝을 지금보다 과거로 둔다.
      final expired = GuideSnapshot(
        tripId: snap.tripId,
        request: snap.request,
        itinerary: snap.itinerary,
        legs: snap.legs,
        legIndex: snap.legIndex,
        shift: snap.shift,
        startedAt: snap.startedAt,
        stayUntil: DateTime.now().subtract(const Duration(minutes: 20)),
      );
      final heard = <String>[];
      final resumed = GuideSession.resume(expired, api: api, speak: (t) async => heard.add(t));
      await resumed.start();
      expect(resumed.staying, isFalse);
      expect(heard.where((t) => t.contains('머무른 뒤')), isEmpty);
      resumed.dispose();
    });

    test('앞 구간으로 되돌리면 체류를 끝낸다', () async {
      final s = await stayAtVia();
      s.prevLeg();
      expect(s.staying, isFalse);
      expect(s.tracker.index, 0);
    });
  });

  group('대중교통에서 내리기 전 탈것 안(이슈 #133)', () {
    // 활동 인식이 IN_VEHICLE 을 준다. 활동 판정은 20초 이어져야 확정되므로 세션 시계를 1분 앞에 두어 첫 위치에서 확정된다.
    Future<GuideSession> ride(List<Map<String, dynamic>> legs) async {
      const actMethod = MethodChannel('flutter_activity_recognition/method');
      const actEvents = EventChannel('flutter_activity_recognition/updates');
      final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
      messenger.setMockMethodCallHandler(actMethod, (_) async => 'GRANTED');
      messenger.setMockStreamHandler(actEvents, MockStreamHandler.inline(onListen: (_, sink) {
        sink.success(jsonEncode({'type': 'IN_VEHICLE', 'confidence': 'HIGH'}));
      }));
      addTearDown(() {
        messenger.setMockMethodCallHandler(actMethod, null);
        messenger.setMockStreamHandler(actEvents, null);
      });
      final s = GuideSession(
        api: api,
        request: _request,
        itinerary: Itinerary.fromJson(_itineraryJson(legs)),
        speak: (t) async => spoken.add(t),
        store: store,
        clock: () => DateTime.now().add(const Duration(minutes: 1)),
      );
      ActiveGuide.instance.set(s);
      await s.start();
      await pumpEventQueue();
      return s;
    }

    Map<String, dynamic> subway(double toLat) =>
        {..._legJson(to: '역', toLat: toLat), 'mode': 'SUBWAY', 'transit_leg': true, 'route': '2호선'};

    test('열차 안 정지로 활동이 미상이 돼도 2분 동안은 지하철 끝점 위치로 넘기지 않고, 그 뒤에는 넘긴다', () async {
      const actMethod = MethodChannel('flutter_activity_recognition/method');
      const actEvents = EventChannel('flutter_activity_recognition/updates');
      final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
      MockStreamHandlerEventSink? events;
      messenger.setMockMethodCallHandler(actMethod, (_) async => 'GRANTED');
      messenger.setMockStreamHandler(actEvents, MockStreamHandler.inline(onListen: (_, sink) {
        events = sink;
        sink.success(jsonEncode({'type': 'IN_VEHICLE', 'confidence': 'HIGH'}));
      }));
      addTearDown(() {
        messenger.setMockMethodCallHandler(actMethod, null);
        messenger.setMockStreamHandler(actEvents, null);
      });
      // 관측 시각은 기기 시계, 확정·감쇠 시각은 세션 시계다. 세션 시계를 기기 시계보다 앞으로 옮겨 가며 잰다.
      final t0 = DateTime.now();
      var clock = t0.add(const Duration(minutes: 1));
      final s = GuideSession(
        api: api,
        request: _request,
        itinerary: Itinerary.fromJson(_itineraryJson([
          subway(37.501),
          {..._legJson(to: '도착지', toLat: 37.502), 'from_lat': 37.501},
        ])),
        speak: (t) async => spoken.add(t),
        store: store,
        clock: () => clock,
      );
      ActiveGuide.instance.set(s);
      await s.start();
      await pumpEventQueue();
      await push(_pos(37.5005, 127.0)); // 지하철 구간 중간 — 탈것 안 확정
      expect(s.activity, 'vehicle');
      events!.success(jsonEncode({'type': 'STILL', 'confidence': 'HIGH'}));
      await pumpEventQueue();
      clock = t0.add(const Duration(minutes: 3));
      await push(_pos(37.5005, 127.0)); // 정지 2분 → 확정 활동 미상
      expect(s.activity, 'unknown');
      clock = t0.add(const Duration(minutes: 4));
      await push(_pos(37.50095, 127.0)); // 지하철 끝점에서 약 6m, 감쇠 뒤 1분
      expect(s.tracker.index, 0);
      clock = t0.add(const Duration(minutes: 5, seconds: 10));
      await push(_pos(37.50095, 127.0)); // 감쇠 뒤 2분 넘음
      expect(s.tracker.index, 1);
    });

    // 활동 인식 모의: IN_VEHICLE 을 먼저 주고, 돌려준 함수로 뒤 판정(STILL 등)을 흘린다.
    void Function(String type) mockActivity() {
      const actMethod = MethodChannel('flutter_activity_recognition/method');
      const actEvents = EventChannel('flutter_activity_recognition/updates');
      final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
      MockStreamHandlerEventSink? events;
      messenger.setMockMethodCallHandler(actMethod, (_) async => 'GRANTED');
      messenger.setMockStreamHandler(actEvents, MockStreamHandler.inline(onListen: (_, sink) {
        events = sink;
        sink.success(jsonEncode({'type': 'IN_VEHICLE', 'confidence': 'HIGH'}));
      }));
      addTearDown(() {
        messenger.setMockMethodCallHandler(actMethod, null);
        messenger.setMockStreamHandler(actEvents, null);
      });
      return (type) => events!.success(jsonEncode({'type': type, 'confidence': 'HIGH'}));
    }

    testWidgets('위치가 끊긴 지하에서 정지로 활동이 미상이 돼도 2분 동안은 시간표로 넘기지 않고, 그 뒤에는 넘긴다', (tester) async {
      final emit = mockActivity();
      // 관측 시각은 기기 시계라 세션 시계를 기기 시계 기준으로 잡는다. 지하철 t0+1분~t0+4분, 위치 표본은 없다.
      final t0 = DateTime.now();
      var now = t0.add(const Duration(minutes: 1));
      final s = await startTicking(tester, [
        {
          ..._legJson(to: '역', toLat: 37.501),
          'mode': 'SUBWAY',
          'transit_leg': true,
          'start': t0.add(const Duration(minutes: 1)).toIso8601String(),
          'end': t0.add(const Duration(minutes: 4)).toIso8601String(),
        },
        {..._legJson(to: '도착지', toLat: 37.502), 'from_lat': 37.501},
      ], () => now);
      await tick(tester); // 탈것 안 확정
      expect(s.activity, 'vehicle');
      emit('STILL');
      await settle(tester);
      now = t0.add(const Duration(minutes: 3));
      await tick(tester); // 정지 2분 → 확정 활동 미상
      expect(s.activity, 'unknown');
      now = t0.add(const Duration(minutes: 4, seconds: 30));
      await tick(tester); // 계획 종료가 지났지만 감쇠 뒤 1분 30초
      expect(s.tracker.index, 0);
      now = t0.add(const Duration(minutes: 5, seconds: 10));
      await tick(tester);
      expect(s.tracker.index, 1);
      ActiveGuide.instance.clear(); // 10초 점검 타이머를 끈다
      await settle(tester);
    });

    test('정지로 미상이 된 뒤 2분 안에는 내려서 걷는 마지막 구간의 목적지 도착으로 끝내지 않고, 그 뒤에는 끝낸다', () async {
      final emit = mockActivity();
      final t0 = DateTime.now();
      var clock = t0.add(const Duration(minutes: 1));
      final s = GuideSession(
        api: api,
        request: _request,
        itinerary: Itinerary.fromJson(_itineraryJson([
          subway(37.501),
          {..._legJson(to: '도착지', toLat: 37.502), 'from_lat': 37.501},
        ])),
        speak: (t) async => spoken.add(t),
        store: store,
        clock: () => clock,
      );
      ActiveGuide.instance.set(s);
      await s.start();
      await pumpEventQueue();
      await push(_pos(37.5005, 127.0));
      expect(s.activity, 'vehicle');
      emit('STILL');
      await pumpEventQueue();
      clock = t0.add(const Duration(minutes: 3));
      await push(_pos(37.5005, 127.0));
      expect(s.activity, 'unknown');
      s.nextLeg(); // 손으로 마지막 도보 구간으로 넘겼다
      clock = t0.add(const Duration(minutes: 3, seconds: 30));
      for (var i = 0; i < 3; i++) {
        await push(_pos(37.50195, 127.0)); // 도착지에서 약 6m
      }
      expect(api.ended, isEmpty);
      clock = t0.add(const Duration(minutes: 5, seconds: 10));
      for (var i = 0; i < 3; i++) {
        await push(_pos(37.50195, 127.0));
      }
      await pumpEventQueue();
      expect(api.ended, ['T1']);
    });

    test('지하철 끝점에 닿은 위치로도 도보 구간으로 넘기지 않는다', () async {
      final s = await ride([subway(37.501), {..._legJson(to: '도착지', toLat: 37.502), 'from_lat': 37.501}]);
      await push(_pos(37.50095, 127.0)); // 지하철 끝점에서 약 6m
      expect(s.activity, 'vehicle');
      expect(s.tracker.index, 0);
    });

    test('내려서 걷는 구간에서 튀는 위치로 경로 이탈 재탐색을 하지 않는다', () async {
      // 지하철(37.50→37.52) → 도보(→37.5215 대여소) → 따릉이(→37.53)
      final s = await ride([
        subway(37.52),
        {..._legJson(to: '대여소', toLat: 37.5215), 'from_lat': 37.52},
        {..._legJson(to: '도착지', toLat: 37.53), 'from_lat': 37.5215, 'mode': 'BICYCLE', 'rented_bike': true},
      ]);
      s.nextLeg(); // 열차 안에서 손으로 도보 구간으로 넘겼다
      for (var i = 0; i < 5; i++) {
        await push(_pos(37.525, 127.0, acc: 49)); // 따릉이 경로선 위로 튄 위치(도보 경로선에서 약 400m)
      }
      for (var i = 0; i < 5; i++) {
        await push(_pos(37.515, 127.0, acc: 30)); // 지하철 선 위(도보 경로선에서 약 560m)
      }
      expect(s.activity, 'vehicle');
      expect(s.tracker.index, 1);
      expect(api.planCalls, 0);
    });

    test('내려서 걷는 마지막 구간에서 도착지 옆 위치로 목적지 도착 자동 종료를 하지 않는다', () async {
      final s = await ride([subway(37.501), {..._legJson(to: '도착지', toLat: 37.502), 'from_lat': 37.501}]);
      s.nextLeg();
      for (var i = 0; i < 3; i++) {
        await push(_pos(37.50195, 127.0)); // 도착지에서 약 6m
      }
      expect(s.ended, isFalse);
      expect(api.ended, isEmpty);
    });

    test('내려서 걷는 구간에서 다음 모퉁이 옆 위치로 도보 안내 단계를 넘기지 않는다', () async {
      final s = await ride([
        subway(37.52),
        {
          ..._legJson(to: '도착지', toLat: 37.5215),
          'from_lat': 37.52,
          'steps': [
            {'dir': 'DEPART', 'distance_m': 100, 'lat': 37.52, 'lon': 127.0},
            {'dir': 'LEFT', 'distance_m': 60, 'lat': 37.5209, 'lon': 127.0},
            {'dir': 'RIGHT', 'distance_m': 60, 'lat': 37.5209, 'lon': 127.0007},
          ],
        },
      ]);
      s.nextLeg();
      final before = s.instr.cueKey;
      await push(_pos(37.5209, 127.0)); // 첫 모퉁이(LEFT) 위
      expect(s.instr.cueKey, before);
    });
  });
}
