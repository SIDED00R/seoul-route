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
}
