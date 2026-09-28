import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/models/result_sort.dart';
import 'package:seoul_route/screens/results_screen.dart';

/// 서버 응답 형식의 여정. 버스 한 구간의 노선 이름([route])으로 구분한다.
Map<String, dynamic> _it(String route, {required double durationSec, double departInSec = 0, int transfers = 0}) => {
      'start': 's',
      'end': 'e',
      'duration_sec': durationSec,
      'depart_in_sec': departInSec,
      'transfers': transfers,
      'walk_distance_m': 0.0,
      'legs': [
        {'mode': 'BUS', 'route': route, 'duration_sec': durationSec, 'distance_m': 1000.0,
         'from_lat': 37.5, 'from_lon': 127.0, 'to_lat': 37.51, 'to_lon': 127.01, 'transit_leg': true},
      ],
    };

List<String> _routes(List<Itinerary> its) => [for (final it in its) it.legs.first.route];

void main() {
  // 서버 순서(추천순): 환승 가산 때문에 66분·62분·60분 순으로 온 실측(2026-09-28 시청→잠실) 모양.
  final serverOrder = PlanResult.fromJson({
    'itineraries': [
      _it('a', durationSec: 66 * 60, transfers: 1),
      _it('b', durationSec: 62 * 60, transfers: 2),
      _it('c', durationSec: 59 * 60, departInSec: 60, transfers: 3), // 대기 포함 60분
      _it('d', durationSec: 54 * 60 + 10, departInSec: 8 * 60, transfers: 2), // 대기 포함 62분, b 와 동률
    ],
  }).itineraries;

  test('추천순은 서버 순서를 그대로 둔다', () {
    expect(_routes(sortItineraries(serverOrder, ResultSort.recommended)), ['a', 'b', 'c', 'd']);
  });

  test('최소시간순은 표시 분(출발 대기 포함) 오름차순, 같은 분이면 서버 순서', () {
    final sorted = sortItineraries(serverOrder, ResultSort.fastest);
    expect(_routes(sorted), ['c', 'b', 'd', 'a']);
    expect([for (final it in sorted) it.minutes], [60, 62, 62, 66]);
    expect(_routes(serverOrder), ['a', 'b', 'c', 'd']); // 원본 목록은 바꾸지 않는다
  });

  test('최소시간순은 후보가 많아도 같은 분끼리 서버 순서를 지킨다', () {
    // List.sort 는 33개 이상에서 안정 정렬이 아니다.
    final many = [
      for (var i = 0; i < 40; i++) Itinerary.fromJson(_it('r$i', durationSec: (60 + i % 3) * 60.0)),
    ];
    final sorted = sortItineraries(many, ResultSort.fastest);
    for (final m in [60, 61, 62]) {
      final same = [for (final it in sorted) if (it.minutes == m) int.parse(it.legs.first.route.substring(1))];
      expect(same, [...same]..sort(), reason: '$m분');
    }
  });

  testWidgets('결과 화면 전환 버튼으로 최소시간순과 추천순을 오간다', (tester) async {
    const p = Place(name: 'A', address: '', lat: 37.5, lon: 127.0);
    await tester.pumpWidget(MaterialApp(
      home: ResultsScreen(
        api: ApiClient(baseUrl: 'http://127.0.0.1:8081', token: 't'),
        request: const PlanRequest(origin: p, destination: p),
        result: PlanResult(itineraries: serverOrder),
      ),
    ));
    List<String> titles() => [
          for (final t in tester.widgetList<ListTile>(find.byType(ListTile))) (t.title as Text).data!.split(' ').first
        ];

    expect(titles(), ['66분', '62분', '60분', '62분']);
    await tester.tap(find.text('최소시간순'));
    await tester.pump();
    expect(titles(), ['60분', '62분', '62분', '66분']);
    await tester.tap(find.text('추천순'));
    await tester.pump();
    expect(titles(), ['66분', '62분', '60분', '62분']);
  });

  testWidgets('경로가 없으면 전환 버튼을 보이지 않는다', (tester) async {
    const p = Place(name: 'A', address: '', lat: 37.5, lon: 127.0);
    await tester.pumpWidget(MaterialApp(
      home: ResultsScreen(
        api: ApiClient(baseUrl: 'http://127.0.0.1:8081', token: 't'),
        request: const PlanRequest(origin: p, destination: p),
        result: const PlanResult(itineraries: [], reason: '경로 없음'),
      ),
    ));
    expect(find.byType(SegmentedButton<ResultSort>), findsNothing);
    expect(find.text('경로 없음'), findsOneWidget);
  });
}
