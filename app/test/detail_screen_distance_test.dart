import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/detail_screen.dart';

Leg _leg(String mode, {bool rentedBike = false, int? bikes}) => Leg(
      mode: mode,
      durationSec: 600,
      distanceM: 4900,
      fromName: '사당',
      toName: '강남',
      fromLat: 37.4766,
      fromLon: 126.9816,
      toLat: 37.4979,
      toLon: 127.0276,
      route: mode == 'SUBWAY' ? '2호선' : '',
      rentedBike: rentedBike,
      transitLeg: mode == 'SUBWAY',
      polyline: '',
      start: '2026-09-21T09:00:00+09:00',
      end: '2026-09-21T09:10:00+09:00',
      bikesAvailable: bikes,
    );

void main() {
  // 탄 구간에는 거리를 안 쓰고 걷는 구간에는 쓴다. 따릉이 구간에는 남은 대수 줄이 붙는다.
  testWidgets('상세 화면: 거리는 도보·자전거만, 따릉이는 남은 대수', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: DetailScreen(
        api: ApiClient(baseUrl: 'http://x', token: 't'),
        request: const PlanRequest(
          origin: Place(name: '사당역', address: '', lat: 37.4766, lon: 126.9816),
          destination: Place(name: '강남역', address: '', lat: 37.4979, lon: 127.0276),
        ),
        itinerary: Itinerary(
          start: '2026-09-21T09:00:00+09:00',
          end: '2026-09-21T09:30:00+09:00',
          durationSec: 1800,
          transfers: 0,
          walkM: 4900,
          legs: [_leg('SUBWAY'), _leg('WALK'), _leg('BICYCLE', rentedBike: true, bikes: 3)],
        ),
        index: 0,
      ),
    ));
    await tester.pump();
    expect(find.text('2호선 · 10분'), findsOneWidget); // km 없음
    expect(find.text('도보 · 10분 · 4.9km'), findsOneWidget);
    expect(find.text('따릉이 · 10분 · 4.9km'), findsOneWidget);
    expect(find.text('남은 자전거 3대'), findsOneWidget);
  });
}
