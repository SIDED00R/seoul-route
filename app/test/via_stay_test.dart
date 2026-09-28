import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/models/recent_route.dart';
import 'package:seoul_route/screens/place_search_screen.dart';
import 'package:seoul_route/screens/plan_screen.dart';
import 'package:seoul_route/settings/settings_store.dart';
import 'package:seoul_route/util/stay_label.dart';
import 'package:seoul_route/widgets/via_stay_picker.dart';

import 'settings_test.dart' show MemoryTokenStorage;

const _a = Place(name: 'A역', address: 'a', lat: 37.55, lon: 126.97);
const _b = Place(name: 'B역', address: 'b', lat: 37.50, lon: 127.03);
const _c = Place(name: 'C역', address: 'c', lat: 37.52, lon: 126.92);

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

/// 경유지 체류 드롭다운을 열고 label 항목을 고른다.
Future<void> _chooseStay(WidgetTester t, String label) async {
  await t.tap(find.byType(DropdownButton<int>));
  await _frames(t);
  await t.tap(find.text(label).last);
  await _frames(t);
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('체류 표기', () {
    expect(stayLabel(0), '바로 통과');
    expect(stayLabel(45), '45분');
    expect(stayLabel(60), '1시간');
    expect(stayLabel(95), '1시간 35분');
  });

  test('경유지 체류·이용권은 요청 JSON 에 실리고 되살아난다. 0 이면 싣지 않는다', () {
    const req = PlanRequest(origin: _a, destination: _b, via: [_c, _a], viaStayMin: [30, 0], bikeLimitMin: 120);
    final j = req.toJson();
    expect((j['via'] as List)[0]['stay_min'], 30);
    expect((j['via'] as List)[1].containsKey('stay_min'), isFalse);
    expect(j['bike_limit_min'], 120);
    expect(req.totalStayMin, 30);

    final back = PlanRequest.fromJson(jsonDecode(jsonEncode(j)) as Map<String, dynamic>);
    expect(back.viaStayMin, [30, 0]);
    expect(back.bikeLimitMin, 120);
    expect(const PlanRequest(origin: _a, destination: _b).toJson().containsKey('bike_limit_min'), isFalse);
  });

  testWidgets('최근 경로의 경유지 체류가 되살아나 길찾기 입력에 채워진다', (tester) async {
    final r = RecentRoute.fromJson({
      'request': {
        'origin': {'lat': 37.55, 'lon': 126.97, 'name': 'A역'},
        'destination': {'lat': 37.50, 'lon': 127.03, 'name': 'B역'},
        'via': [
          {'lat': 37.52, 'lon': 126.92, 'name': 'C역', 'stay_min': 40},
          {'lat': 37.53, 'lon': 126.95, 'name': 'D역'},
        ],
      },
      'searched_at': '2026-09-28T08:16:00Z',
    });
    expect(r.request.viaStayMin, [40, 0]);
    await tester.pumpWidget(MaterialApp(
      home: PlanScreen(
        settings: const Settings(baseUrl: 'http://127.0.0.1:8081', token: 't'),
        preset: PlanScreenPreset(r.request),
      ),
    ));
    await _frames(tester);
    expect(find.text('40분'), findsOneWidget);
    expect(find.text('바로 통과'), findsOneWidget);
  });

  test('이용권 설정은 저장·복원되고 60·120 밖의 값은 1시간권으로 본다', () async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    final store = SettingsStore(tokenStorage: MemoryTokenStorage());
    expect((await store.load()).bikeLimitMin, 60);
    await store.save(const Settings(baseUrl: 'http://x', token: 't', bikeLimitMin: 120));
    expect((await store.load()).bikeLimitMin, 120);
    SharedPreferences.setMockInitialValues(<String, Object>{'bike_limit_min': 90});
    expect((await store.load()).bikeLimitMin, 60);
  });

  testWidgets('경유지 체류를 고르거나 직접 넣으면 요청에 실리고, 설정의 이용권도 함께 간다', (tester) async {
    final bodies = <Map<String, dynamic>>[];
    await http.runWithClient(() async {
      await tester.pumpWidget(MaterialApp(
        home: PlanScreen(
          settings: const Settings(baseUrl: 'http://127.0.0.1:8081', token: 't', bikeLimitMin: 120),
          locate: (_) async => _a,
        ),
      ));
      await _pick(tester, find.text('출발지 선택'), _a);
      await _pick(tester, find.text('도착지 선택'), _b);
      await _pick(tester, find.text('경유지 추가'), _c);
      expect(find.text('바로 통과'), findsOneWidget);

      await _chooseStay(tester, '30분');
      await tester.tap(find.byType(FilledButton));
      await _frames(tester);
      expect(bodies.last['via'][0]['stay_min'], 30);
      expect(bodies.last['bike_limit_min'], 120);
      Navigator.pop(tester.element(find.text('소요 시간에 경유지 체류 30분이 들어 있습니다')));
      await _frames(tester);

      await _chooseStay(tester, '직접 입력…');
      await tester.enterText(find.byType(TextField), '75');
      await tester.pump();
      await tester.tap(find.text('확인'));
      await _frames(tester);
      expect(find.text('1시간 15분'), findsOneWidget);
      await tester.tap(find.byType(FilledButton));
      await _frames(tester);
      expect(bodies.last['via'][0]['stay_min'], 75);
    }, () => MockClient((r) async {
          bodies.add(jsonDecode(r.body) as Map<String, dynamic>);
          return http.Response('{"itineraries":[{"start":"s","end":"e","duration_sec":600,"legs":[]}]}', 200,
              headers: {'content-type': 'application/json'});
        }));
  });

  testWidgets('직접 입력은 1~180분만 받는다', (tester) async {
    int? got = -1;
    await tester.pumpWidget(MaterialApp(
      home: Builder(
        builder: (context) => TextButton(
          onPressed: () async => got = await askStayMinutes(context, 0),
          child: const Text('열기'),
        ),
      ),
    ));
    await tester.tap(find.text('열기'));
    await tester.pumpAndSettle();
    FilledButton ok() => tester.widget<FilledButton>(find.widgetWithText(FilledButton, '확인'));
    expect(ok().onPressed, isNull); // 비어 있으면 확인 불가
    await tester.enterText(find.byType(TextField), '181');
    await tester.pump();
    expect(ok().onPressed, isNull);
    expect(find.text('1~180분 사이로 넣어 주세요'), findsOneWidget);
    await tester.enterText(find.byType(TextField), '0');
    await tester.pump();
    expect(ok().onPressed, isNull);
    await tester.enterText(find.byType(TextField), '180');
    await tester.pump();
    await tester.tap(find.text('확인'));
    await tester.pumpAndSettle();
    expect(got, 180);
  });
}
