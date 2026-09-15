import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/results_screen.dart';

/// 서버 응답 형식의 여정 하나. replanned 가 null 이면 키를 넣지 않는다(구버전 서버 응답).
Map<String, dynamic> _it({bool? replanned}) {
  final m = <String, dynamic>{
    'start': 's',
    'end': 'e',
    'duration_sec': 1500,
    'crossing_wait_sec': 114,
    'legs': [
      {'mode': 'WALK', 'duration_sec': 300, 'distance_m': 400, 'from_lat': 37.55, 'from_lon': 126.97,
       'to_lat': 37.551, 'to_lon': 126.971, 'transit_leg': false},
    ],
  };
  if (replanned != null) m['replanned'] = replanned;
  return m;
}

void main() {
  test('replanned 가 true 일 때만 재탐색 배지 문구', () {
    expect(Itinerary.fromJson(_it(replanned: true)).replannedLabel, '재탐색');
    expect(Itinerary.fromJson(_it(replanned: false)).replannedLabel, isNull);
    expect(Itinerary.fromJson(_it()).replannedLabel, isNull);
  });

  testWidgets('결과 목록은 재탐색 여정에 배지를 붙인다', (tester) async {
    final result = PlanResult.fromJson({
      'itineraries': [_it(replanned: true), _it(replanned: false)],
    });
    const p = Place(name: 'A역', address: '', lat: 37.55, lon: 126.97);
    await tester.pumpWidget(MaterialApp(
      home: ResultsScreen(
        api: ApiClient(baseUrl: 'http://127.0.0.1:8081', token: 't'),
        request: const PlanRequest(origin: p, destination: p),
        result: result,
      ),
    ));
    expect(find.text('재탐색'), findsOneWidget);
    expect(find.text('횡단보도 +2분'), findsNWidgets(2));
  });
}
