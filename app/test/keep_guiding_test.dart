import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/guide/active_guide.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/current_guide_tab.dart';
import 'package:seoul_route/screens/detail_screen.dart';
import 'package:seoul_route/screens/guide_screen.dart';

/// 위치 스트림에 표본을 밀어 넣는 가짜 위치 제공자.
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

  int startCalls = 0;
  int endCalls = 0;

  /// 업로드가 실패하게 한다 — 종료가 못 보낸 샘플을 남기고 null 을 돌려주는 상황.
  bool uploadFails = false;

  /// 종료 응답을 붙잡아 둔다. release() 로 놓아 준다.
  bool holdEnd = false;
  Completer<Map<String, dynamic>>? _held;

  void release() => _held?.complete(const {});

  @override
  Future<String> startTrip() async {
    startCalls++;
    return 'T$startCalls';
  }

  @override
  Future<void> uploadTraces(String tripId, List samples) async {
    if (uploadFails) throw ApiException(503, '서버 없음');
  }

  @override
  Future<Map<String, dynamic>> endTrip(String tripId) async {
    endCalls++;
    if (!holdEnd) return const {};
    final c = _held = Completer<Map<String, dynamic>>();
    return c.future;
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

Leg _walk(double toLat) => Leg(
      mode: 'WALK',
      durationSec: 300,
      distanceM: 222,
      fromName: 'Origin',
      toName: 'Destination',
      fromLat: 37.5,
      fromLon: 127.0,
      toLat: toLat,
      toLon: 127.0,
      route: '',
      rentedBike: false,
      transitLeg: false,
      polyline: '',
      start: _t(0),
      end: _t(5),
    );

Itinerary _itin(double toLat) => Itinerary(
      start: _t(0),
      end: _t(5),
      durationSec: 300,
      transfers: 0,
      walkM: 222,
      legs: [_walk(toLat)],
    );

const _request = PlanRequest(
  origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착', address: '', lat: 37.502, lon: 127.0),
);

/// 테스트를 끝내며 안내를 치운다. 안내는 화면과 별개로 살아 있어(10초 타이머) 치우지 않으면 테스트가 실패한다.
Future<void> finish(WidgetTester tester) async {
  ActiveGuide.instance.clear();
  await tester.pumpWidget(const SizedBox());
  await tester.pump(const Duration(seconds: 10));
}

Future<void> _settle(WidgetTester tester, {int rounds = 6}) async {
  for (var i = 0; i < rounds; i++) {
    await tester.pump(const Duration(milliseconds: 10));
  }
}

/// 조건이 될 때까지 기다린다. 안내 종료는 플랫폼 채널·업로더를 거치므로 가짜 시계 pump 만으로는 끝나지 않아
/// runAsync 로 실제 비동기를 흘린다.
Future<void> _waitFor(WidgetTester tester, bool Function() done, {int rounds = 50}) async {
  for (var i = 0; i < rounds && !done(); i++) {
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 20)));
    await tester.pump();
  }
}

void main() {
  const permission = MethodChannel('seoul_route/notification_permission');
  const status = MethodChannel('seoul_route/guide_status');
  late _Geo geo;
  late _Api api;

  setUp(() {
    ActiveGuide.instance.clear();
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(permission, (call) async => true);
    messenger.setMockMethodCallHandler(status, (call) async => null);
    geo = _Geo();
    GeolocatorPlatform.instance = geo;
    api = _Api();
  });

  tearDown(() {
    ActiveGuide.instance.clear();
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(permission, null);
    messenger.setMockMethodCallHandler(status, null);
  });

  // 뒤로가기로 안내 화면을 닫아도 안내는 끝나지 않는다 — 위치도 계속 받고 trip 도 닫지 않는다.
  testWidgets('안내 화면을 닫아도 안내는 이어진다', (tester) async {
    final itinerary = _itin(37.502);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Builder(
          builder: (context) => TextButton(
            onPressed: () => Navigator.push(
              context,
              MaterialPageRoute(
                builder: (_) => GuideScreen(api: api, request: _request, itinerary: itinerary, speak: (_) async {}),
              ),
            ),
            child: const Text('안내 시작'),
          ),
        ),
      ),
    ));
    await tester.tap(find.text('안내 시작'));
    await _settle(tester);
    expect(api.startCalls, 1);
    final session = ActiveGuide.instance.current;
    expect(session, isNotNull);

    // 뒤로가기
    await tester.pageBack();
    await _settle(tester);
    expect(find.text('안내 시작'), findsOneWidget); // 홈으로 돌아왔다
    expect(ActiveGuide.instance.current, same(session)); // 안내는 그대로
    expect(api.endCalls, 0); // trip 을 닫지 않았다

    // 화면이 없어도 위치를 계속 받는다.
    geo.controller.add(_pos(37.5005, 127.0));
    await _settle(tester);
    expect(session!.samples, 1);
    await finish(tester);
  });

  // "현재 경로" 탭은 진행 중인 안내를 보여 주고 안내 화면으로 돌려보낸다. 안내가 없으면 비어 있다.
  testWidgets('현재 경로 탭: 안내가 없으면 비고, 있으면 다시 열 수 있다', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: CurrentGuideTab())));
    await tester.pump();
    expect(find.textContaining('안내 중인 경로가 없습니다'), findsOneWidget);

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: GuideScreen(api: api, request: _request, itinerary: _itin(37.502), speak: (_) async {}),
      ),
    ));
    await _settle(tester);
    final session = ActiveGuide.instance.current;

    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: CurrentGuideTab())));
    await _settle(tester);
    expect(find.text('안내 화면으로'), findsOneWidget);
    expect(find.textContaining('출발 → 도착'), findsOneWidget);

    await tester.tap(find.text('안내 화면으로'));
    await _settle(tester);
    expect(find.text('안내 종료'), findsOneWidget); // 안내 화면이 열렸다
    expect(api.startCalls, 1); // 다시 시작하지 않고 하던 안내를 이어받았다
    expect(ActiveGuide.instance.current, same(session));
    await finish(tester);
  });

  // 다른 여정으로 안내를 시작하려 하면 묻고, 승낙해야 하던 안내를 끝내고 새로 시작한다.
  testWidgets('안내 중 다른 여정을 시작하려면 묻는다', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: GuideScreen(api: api, request: _request, itinerary: _itin(37.502), speak: (_) async {}),
      ),
    ));
    await _settle(tester);
    final first = ActiveGuide.instance.current;
    expect(api.startCalls, 1);

    await tester.pumpWidget(MaterialApp(
      home: DetailScreen(api: api, request: _request, itinerary: _itin(37.503), index: 0),
    ));
    await tester.pump();
    await tester.tap(find.text('안내 시작'));
    await tester.pumpAndSettle();
    expect(find.text('하던 안내를 끝내고 새로 시작할까요?'), findsOneWidget);

    await tester.tap(find.text('계속 안내'));
    await tester.pumpAndSettle();
    expect(ActiveGuide.instance.current, same(first)); // 하던 안내 그대로
    expect(api.endCalls, 0);

    await tester.tap(find.text('안내 시작'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('새로 시작'));
    // 하던 안내를 닫고(샘플 전송·trip 종료) 새 안내가 시작될 때까지 기다린다.
    await _waitFor(tester, () => api.startCalls == 2);
    expect(api.endCalls, 1); // 하던 안내를 닫고
    expect(api.startCalls, 2); // 새로 시작했다
    expect(ActiveGuide.instance.current, isNot(same(first)));
    await finish(tester);
  });

  // 하던 안내를 끝내지 못했으면(샘플 전송 실패) 그대로 교체하지 않는다 — 치우면 못 보낸 샘플이 사라지고 서버 trip 도
  // 열린 채 남는다. 대신 버리고 갈 길은 남겨 둔다(서버에 못 닿는 동안 새 안내를 아예 못 하게 되면 안 된다).
  testWidgets('종료에 실패하면 묻고, 버리기를 고를 때만 교체한다', (tester) async {
    api.uploadFails = true;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: GuideScreen(api: api, request: _request, itinerary: _itin(37.502), speak: (_) async {}),
      ),
    ));
    await _settle(tester);
    final first = ActiveGuide.instance.current;
    geo.controller.add(_pos(37.5005, 127.0)); // 올리지 못할 샘플 하나
    await _settle(tester);

    await tester.pumpWidget(MaterialApp(
      home: DetailScreen(api: api, request: _request, itinerary: _itin(37.503), index: 0),
    ));
    await tester.pump();
    await tester.tap(find.text('안내 시작'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('새로 시작'));
    await _waitFor(tester, () => find.text('하던 안내를 끝내지 못했습니다').evaluate().isNotEmpty);
    expect(find.text('하던 안내를 끝내지 못했습니다'), findsOneWidget);

    await tester.tap(find.text('취소'));
    await tester.pumpAndSettle();
    expect(ActiveGuide.instance.current, same(first)); // 하던 안내 그대로
    expect(api.startCalls, 1); // 새로 시작하지 않았다

    await tester.tap(find.text('안내 시작'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('새로 시작'));
    await _waitFor(tester, () => find.text('버리고 시작').evaluate().isNotEmpty);
    await tester.tap(find.text('버리고 시작'));
    await _waitFor(tester, () => api.startCalls == 2);
    expect(api.startCalls, 2);
    expect(ActiveGuide.instance.current, isNot(same(first)));
    await finish(tester);
  });

  // 종료 응답을 기다리는 동안 화면을 떠나도, 성공한 종료는 전역에서 치워야 한다(끝난 세션과 활동 인식 구독이 남는다).
  testWidgets('화면을 떠난 뒤 종료가 끝나도 안내를 치운다', (tester) async {
    tester.view.physicalSize = const Size(1400, 2400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    api.holdEnd = true;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: GuideScreen(api: api, request: _request, itinerary: _itin(37.502), speak: (_) async {}),
      ),
    ));
    await _settle(tester);
    final session = ActiveGuide.instance.current;

    await tester.tap(find.text('안내 종료'));
    await _waitFor(tester, () => api.endCalls == 1);
    expect(api.endCalls, 1); // 서버 응답을 기다리는 중

    // 응답이 오기 전에 화면을 떠난다.
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: CurrentGuideTab())));
    await _settle(tester);
    expect(ActiveGuide.instance.current, same(session));

    api.release(); // 서버 응답 도착
    await _waitFor(tester, () => ActiveGuide.instance.current == null);
    expect(ActiveGuide.instance.current, isNull);
    await finish(tester);
  });
}
