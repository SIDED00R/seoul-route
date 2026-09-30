import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:geolocator/geolocator.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/location/current_location.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/models/plan_time.dart';
import 'package:seoul_route/screens/detail_screen.dart';
import 'package:seoul_route/screens/place_search_screen.dart';
import 'package:seoul_route/screens/plan_screen.dart';
import 'package:seoul_route/screens/results_screen.dart';
import 'package:seoul_route/settings/settings_store.dart';
import 'package:seoul_route/widgets/plan_time_picker.dart';

const _settings = Settings(baseUrl: 'http://127.0.0.1:8081', token: 't');
const _a = Place(name: 'A역', address: 'a', lat: 37.55, lon: 126.97);
const _b = Place(name: 'B역', address: 'b', lat: 37.50, lon: 127.03);
const _c = Place(name: 'C역', address: 'c', lat: 37.52, lon: 126.92);
const _d = Place(name: 'D역', address: 'd', lat: 37.53, lon: 126.95);
const _here = Place(name: '현재 위치', address: '정확도 약 12m', lat: 37.5006, lon: 127.0364);

Future<void> _frames(WidgetTester t) async {
  for (var i = 0; i < 10; i++) {
    await t.pump(const Duration(milliseconds: 100));
  }
}

Future<void> _pick(WidgetTester t, Finder opener, Place p) async {
  await t.tap(opener);
  await _frames(t);
  Navigator.pop(t.element(find.byType(PlaceSearchScreen)), p);
  await _frames(t);
}

/// index 번째 경유지의 체류 드롭다운을 열고 label 항목을 고른다.
Future<void> _chooseStay(WidgetTester t, int index, String label) async {
  await t.tap(find.byType(DropdownButton<int>).at(index));
  await _frames(t);
  await t.tap(find.text(label).last);
  await _frames(t);
}

/// 경유지·시각 줄이 늘어도 경로 찾기 버튼까지 한 화면에 들어오게 화면을 키운다(목록 밖 위젯은 만들어지지 않는다).
void _tall(WidgetTester t) {
  t.view.physicalSize = const Size(1440, 4000);
  t.view.devicePixelRatio = 1;
  addTearDown(t.view.reset);
}

/// 권한이 없는 기기. requestPermission 을 부르면 센다.
class _DeniedGeolocator extends GeolocatorPlatform {
  int asked = 0;

  @override
  Future<bool> isLocationServiceEnabled() async => true;

  @override
  Future<LocationPermission> checkPermission() async => LocationPermission.denied;

  @override
  Future<LocationPermission> requestPermission() async {
    asked++;
    return LocationPermission.denied;
  }
}

void main() {
  test('출발·도착 시각은 UTC RFC3339 로 보내고 되살린다', () {
    final at = DateTime(2026, 10, 5, 9, 0);
    final req = PlanRequest(origin: _a, destination: _b, arrive: at);
    final j = req.toJson();
    expect(j['arrive'], at.toUtc().toIso8601String());
    expect((j['arrive'] as String).endsWith('Z'), isTrue);
    expect(j.containsKey('depart'), isFalse);
    expect(PlanRequest.fromJson(j).arrive, at);
    expect(PlanRequest(origin: _a, destination: _b).toJson().containsKey('arrive'), isFalse);
  });

  test('시각 문구: 오늘·내일·그 밖', () {
    final today = DateTime(2026, 10, 4, 22, 0);
    expect(PlanTime.now.label(today), '지금 출발');
    expect(PlanTime(PlanTimeKind.depart, DateTime(2026, 10, 4, 23, 5)).label(today), '23:05 출발');
    expect(PlanTime(PlanTimeKind.arrive, DateTime(2026, 10, 5, 9, 0)).label(today), '내일 09:00 도착');
    expect(PlanTime(PlanTimeKind.arrive, DateTime(2026, 10, 7, 9, 0)).label(today), '10월 7일(수) 09:00 도착');
  });

  testWidgets('출발 시각 기본값은 지금 이후의 5분 단위', (tester) async {
    for (final (now, want) in [
      (DateTime(2026, 9, 29, 10, 5, 30), DateTime(2026, 9, 29, 10, 10)),
      (DateTime(2026, 9, 29, 10, 5), DateTime(2026, 9, 29, 10, 5)),
      (DateTime(2026, 9, 29, 10, 6, 1), DateTime(2026, 9, 29, 10, 10)),
    ]) {
      PlanTime? got;
      await tester.pumpWidget(MaterialApp(
          home: Scaffold(
              body: PlanTimePicker(value: PlanTime.now, now: () => now, onChanged: (t) => got = t))));
      await tester.tap(find.text('출발 시각'));
      expect(got?.at, want, reason: '$now');
    }
  });

  test('화면을 열 때 쓰는 현재 위치는 권한을 묻지 않는다', () async {
    final geo = _DeniedGeolocator();
    GeolocatorPlatform.instance = geo;
    await expectLater(quietCurrentPlace(ApiClient(baseUrl: 'http://x', token: 't')), throwsA(isA<LocationException>()));
    expect(geo.asked, 0);
    await expectLater(currentPlace(ApiClient(baseUrl: 'http://x', token: 't')), throwsA(isA<LocationException>()));
    expect(geo.asked, 1); // 버튼으로 받을 때는 묻는다
  });

  testWidgets('출발·도착 바꾸기: 경유지·체류·구간 수단 순서도 뒤집는다', (tester) async {
    _tall(tester);
    Map<String, dynamic>? sent;
    final mock = MockClient((r) async {
      sent = jsonDecode(r.body) as Map<String, dynamic>;
      return http.Response('{"itineraries":[]}', 200, headers: {'content-type': 'application/json'});
    });
    await http.runWithClient(() async {
      await tester.pumpWidget(MaterialApp(home: PlanScreen(settings: _settings, locate: (_) async => _a)));
      await _pick(tester, find.text('출발지 선택'), _a);
      await _pick(tester, find.text('도착지 선택'), _b);
      await _pick(tester, find.text('경유지 추가'), _c);
      await _pick(tester, find.text('경유지 추가'), _d);
      await _chooseStay(tester, 0, '30분'); // C역 30분
      await _chooseStay(tester, 1, '1시간'); // D역 60분
      await tester.tap(find.text('도보').first); // 구간 1(A→C) 도보
      await tester.pump();
      await tester.tap(find.byTooltip('출발·도착 바꾸기'));
      await tester.pump();
      expect(find.text('출발: B역'), findsOneWidget);
      expect(find.text('도착: A역'), findsOneWidget);
      expect(find.text('경유 1: D역'), findsOneWidget);
      expect(find.text('경유 2: C역'), findsOneWidget);

      await tester.tap(find.byType(FilledButton));
      await _frames(tester);
      expect(sent!['segment_modes'], ['any', 'any', 'walk']); // 도보 구간은 이제 마지막(C→A)
      final via = sent!['via'] as List;
      expect([for (final v in via) v['name']], ['D역', 'C역']);
      expect([for (final v in via) v['stay_min']], [60, 30]); // 체류는 경유지를 따라간다
    }, () => mock);
  });

  testWidgets('화면을 열면 출발지를 현재 위치로 채우고, 그 사이 직접 고른 출발지는 덮지 않는다', (tester) async {
    await tester.pumpWidget(MaterialApp(
        home: PlanScreen(settings: _settings, locate: (_) async => _a, locateOnOpen: (_) async => _here)));
    await _frames(tester);
    expect(find.text('출발: 현재 위치'), findsOneWidget);

    final late = Completer<Place>();
    await tester.pumpWidget(MaterialApp(
        home: PlanScreen(
            key: UniqueKey(), settings: _settings, locate: (_) async => _a, locateOnOpen: (_) => late.future)));
    await tester.pump();
    expect(find.text('현재 위치 찾는 중…'), findsOneWidget);
    await _pick(tester, find.text('현재 위치 찾는 중…'), _b); // 받는 동안에도 고를 수 있다
    late.complete(_here);
    await _frames(tester);
    expect(find.text('출발: B역'), findsOneWidget);
  });

  testWidgets('도착 시각: 요청에 arrive 로 가고, 경유지가 생기면 같은 시각의 출발 시각으로 바뀐다', (tester) async {
    _tall(tester);
    Map<String, dynamic>? sent;
    final mock = MockClient((r) async {
      sent = jsonDecode(r.body) as Map<String, dynamic>;
      return http.Response('{"itineraries":[]}', 200, headers: {'content-type': 'application/json'});
    });
    await http.runWithClient(() async {
      await tester.pumpWidget(MaterialApp(home: PlanScreen(settings: _settings, locate: (_) async => _a)));
      await _pick(tester, find.text('출발지 선택'), _a);
      await _pick(tester, find.text('도착지 선택'), _b);
      await tester.tap(find.text('도착 시각'));
      await tester.pump();
      expect(find.textContaining('도착 경로 찾기'), findsOneWidget);
      await tester.tap(find.byType(FilledButton));
      await _frames(tester);
      expect(sent!.containsKey('arrive'), isTrue);
      expect(sent!.containsKey('depart'), isFalse);
      final arrive = DateTime.parse(sent!['arrive'] as String);
      await tester.pageBack();
      await _frames(tester);

      await _pick(tester, find.text('경유지 추가'), _c);
      expect(find.textContaining('출발 경로 찾기'), findsOneWidget);
      expect(find.text('경유지가 있거나 구간 수단을 고정하면 도착 시각은 고를 수 없습니다'), findsOneWidget);
      await tester.tap(find.byType(FilledButton));
      await _frames(tester);
      expect(sent!.containsKey('arrive'), isFalse);
      expect(DateTime.parse(sent!['depart'] as String), arrive);
    }, () => mock);
  });

  testWidgets('최근 경로를 고르면 시각 조건이 지금 출발로 돌아간다', (tester) async {
    Widget screen(PlanScreenPreset? preset) => MaterialApp(
        home: PlanScreen(key: const ValueKey('plan'), settings: _settings, locate: (_) async => _a, preset: preset));
    await tester.pumpWidget(screen(null));
    await tester.tap(find.text('도착 시각'));
    await tester.pump();
    expect(find.textContaining('도착 경로 찾기'), findsOneWidget);
    await tester.pumpWidget(screen(const PlanScreenPreset(PlanRequest(origin: _a, destination: _b))));
    await tester.pump();
    expect(find.text('출발: A역'), findsOneWidget);
    expect(find.text('지금 출발 경로 찾기'), findsOneWidget);
  });

  testWidgets('설정이 나중에 채워지면 그때 출발지를 현재 위치로 채운다', (tester) async {
    Widget screen(Settings s) => MaterialApp(
        home: PlanScreen(
            key: const ValueKey('plan'), settings: s, locate: (_) async => _a, locateOnOpen: (_) async => _here));
    await tester.pumpWidget(screen(const Settings(baseUrl: 'http://127.0.0.1:8081', token: '')));
    await _frames(tester);
    expect(find.text('출발지 선택'), findsOneWidget);
    await tester.pumpWidget(screen(_settings));
    await _frames(tester);
    expect(find.text('출발: 현재 위치'), findsOneWidget);
  });

  testWidgets('고른 시각이 지나면 검색하지 않고 다시 고르라고 한다', (tester) async {
    _tall(tester);
    var clock = DateTime(2026, 9, 29, 10, 0, 30);
    var calls = 0;
    final mock = MockClient((r) async {
      if (r.url.path.endsWith('/routes/plan')) calls++;
      return http.Response('{"itineraries":[]}', 200, headers: {'content-type': 'application/json'});
    });
    await http.runWithClient(() async {
      await tester.pumpWidget(
          MaterialApp(home: PlanScreen(settings: _settings, locate: (_) async => _a, now: () => clock)));
      await _pick(tester, find.text('출발지 선택'), _a);
      await _pick(tester, find.text('도착지 선택'), _b);
      await tester.tap(find.text('출발 시각'));
      await tester.pump();
      expect(find.text('10:05 출발 경로 찾기'), findsOneWidget);
      clock = DateTime(2026, 9, 29, 10, 6); // 화면을 켜 둔 사이에 지났다
      await tester.tap(find.byType(FilledButton));
      await _frames(tester);
      expect(find.text('지난 시각입니다 — 출발·도착 시각을 다시 고르세요'), findsOneWidget);
      expect(calls, 0);
    }, () => mock);
  });

  testWidgets('결과에 출발·도착 시각, 상세에 방면·정거장 수와 구간 시각', (tester) async {
    final leg = Leg.fromJson({
      'mode': 'SUBWAY', 'duration_sec': 720, 'distance_m': 5000, 'from_name': '왕십리(2호선)', 'to_name': '을지로입구',
      'from_lat': 37.561, 'from_lon': 127.037, 'to_lat': 37.566, 'to_lon': 126.982, 'route': '2호선',
      'transit_leg': true, 'start': '2026-09-29T14:05:00+09:00', 'end': '2026-09-29T14:17:00+09:00',
      'headsign': '외선순환 시청·홍대입구',
      'stops': [
        for (final n in ['상왕십리', '신당', '동대문역사문화공원', '을지로4가', '을지로3가']) {'name': n, 'lat': 37.5, 'lon': 127.0},
      ],
    });
    final it = Itinerary(start: '2026-09-29T14:02:00+09:00', end: '2026-09-29T14:20:00+09:00', durationSec: 1080,
        transfers: 0, walkM: 300, legs: [leg]);
    const req = PlanRequest(origin: _a, destination: _b);
    final api = ApiClient(baseUrl: 'http://x', token: 't');
    await tester.pumpWidget(
        MaterialApp(home: ResultsScreen(api: api, request: req, result: PlanResult(itineraries: [it]))));
    expect(find.text('14:02 출발 → 14:20 도착'), findsOneWidget);

    await tester.pumpWidget(MaterialApp(home: DetailScreen(api: api, request: req, itinerary: it, index: 1)));
    await tester.pump();
    expect(find.text('외선순환 시청·홍대입구 방면 · 6정거장'), findsOneWidget);
    expect(find.text('왕십리(2호선) 14:05 → 을지로입구 14:17'), findsOneWidget);
  });
}
