import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/models/fast_exit.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/detail_screen.dart';

Leg _subway({List<FastExitFacility> fastExit = const []}) => Leg(
      mode: 'SUBWAY',
      durationSec: 600,
      distanceM: 4900,
      fromName: '사당(2호선)',
      toName: '강남(2호선)',
      fromLat: 37.4766,
      fromLon: 126.9816,
      toLat: 37.4979,
      toLon: 127.0276,
      route: '2호선',
      rentedBike: false,
      transitLeg: true,
      polyline: '',
      start: '2026-09-21T09:00:00+09:00',
      end: '2026-09-21T09:10:00+09:00',
      fastExit: fastExit,
    );

void main() {
  // 상세 화면의 지하철 구간 줄에 하차역 설비 앞 칸이 보이고, 자료가 없는 구간에는 그 줄이 없다.
  testWidgets('상세 화면은 지하철 구간에 하차역 설비 앞 칸을 보여 준다', (tester) async {
    final itinerary = Itinerary(
      start: '2026-09-21T09:00:00+09:00',
      end: '2026-09-21T09:20:00+09:00',
      durationSec: 1200,
      transfers: 0,
      walkM: 0,
      legs: [
        _subway(fastExit: const [
          FastExitFacility(name: '계단', doors: ['2-4', '4-3']),
          FastExitFacility(name: '엘리베이터', doors: ['8-1']),
        ]),
        _subway(),
      ],
    );
    await tester.pumpWidget(MaterialApp(
      home: DetailScreen(
        api: ApiClient(baseUrl: 'http://x', token: 't'),
        request: const PlanRequest(
          origin: Place(name: '사당역', address: '', lat: 37.4766, lon: 126.9816),
          destination: Place(name: '강남역', address: '', lat: 37.4979, lon: 127.0276),
        ),
        itinerary: itinerary,
        index: 0,
      ),
    ));
    await tester.pump();
    expect(find.text('내릴 때 · 계단 2-4, 4-3 · 엘리베이터 8-1'), findsOneWidget);
    expect(find.textContaining('내릴 때'), findsOneWidget);
  });
}
