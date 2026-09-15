import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/place_search_screen.dart';
import 'package:seoul_route/screens/plan_screen.dart';
import 'package:seoul_route/settings/settings_store.dart';

const _settings = Settings(baseUrl: 'http://127.0.0.1:8081', token: 't');
const _a = Place(name: 'A역', address: 'a', lat: 37.55, lon: 126.97);
const _b = Place(name: 'B역', address: 'b', lat: 37.50, lon: 127.03);
const _c = Place(name: 'C역', address: 'c', lat: 37.52, lon: 126.92);

// 진행 표시가 도는 동안은 pumpAndSettle 이 끝나지 않으므로 프레임을 몇 번 넘긴다.
Future<void> _frames(WidgetTester t) async {
  for (var i = 0; i < 10; i++) {
    await t.pump(const Duration(milliseconds: 100));
  }
}

/// 장소 줄·버튼을 눌러 검색 화면을 열고 그 화면이 결과를 고른 것처럼 p 를 돌려준다.
Future<void> _pick(WidgetTester t, Finder opener, Place p) async {
  await t.tap(opener);
  await _frames(t);
  Navigator.pop(t.element(find.byType(PlaceSearchScreen)), p);
  await _frames(t);
}

SegmentedButton<SegmentMode> _modes(WidgetTester t, int i) => t.widget<SegmentedButton<SegmentMode>>(
    find.byWidgetPredicate((w) => w is SegmentedButton<SegmentMode>).at(i));

void main() {
  testWidgets('경로 요청 중에는 도착지·경유지 추가·삭제·구간 수단을 바꿀 수 없고, 끝나면 다시 된다', (tester) async {
    final reply = Completer<http.Response>();
    await http.runWithClient(() async {
      await tester.pumpWidget(MaterialApp(home: PlanScreen(settings: _settings, locate: () async => _a)));
      await _pick(tester, find.text('출발지 선택'), _a);
      await _pick(tester, find.text('도착지 선택'), _b);
      await _pick(tester, find.text('경유지 추가'), _c);
      expect(find.text('경유 1: C역'), findsOneWidget);
      TextButton addVia() => tester.widget<TextButton>(find.widgetWithText(TextButton, '경유지 추가'));
      IconButton removeVia() => tester.widget<IconButton>(find.widgetWithIcon(IconButton, Icons.close));
      expect(addVia().onPressed, isNotNull);
      expect(removeVia().onPressed, isNotNull);
      expect(_modes(tester, 0).onSelectionChanged, isNotNull);

      await tester.tap(find.byType(FilledButton));
      await tester.pump();
      // 요청 중: 도착지 줄을 눌러도 검색이 열리지 않고, 경유지 추가·삭제와 두 구간의 수단 버튼이 꺼진다.
      await tester.tap(find.text('도착: B역'));
      await _frames(tester);
      expect(find.byType(PlaceSearchScreen), findsNothing);
      expect(addVia().onPressed, isNull);
      expect(removeVia().onPressed, isNull);
      expect(_modes(tester, 0).onSelectionChanged, isNull);
      expect(_modes(tester, 1).onSelectionChanged, isNull);

      reply.complete(http.Response('{"error":"x"}', 500, headers: {'content-type': 'application/json'}));
      await _frames(tester);
      expect(addVia().onPressed, isNotNull);
      expect(removeVia().onPressed, isNotNull);
      expect(_modes(tester, 1).onSelectionChanged, isNotNull);
      await _pick(tester, find.text('도착: B역'), _c); // 요청이 끝나면 도착지를 검색으로 바꿀 수 있다
      expect(find.text('도착: C역'), findsOneWidget);
    }, () => MockClient((_) => reply.future));
  });
}
