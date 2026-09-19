import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/guide/active_guide.dart';
import 'package:seoul_route/guide/guide_session.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/current_guide_tab.dart';

class _Api extends ApiClient {
  _Api() : super(baseUrl: 'http://x', token: 't');
}

final _base = DateTime.now();
String _t(int m) => _base.add(Duration(minutes: m)).toIso8601String();

Leg _walk(int from, int to) => Leg(
      mode: 'WALK',
      durationSec: 300,
      distanceM: 222,
      fromName: 'Origin',
      toName: 'Destination',
      fromLat: 37.5,
      fromLon: 127.0,
      toLat: 37.502,
      toLon: 127.0,
      route: '',
      rentedBike: false,
      transitLeg: false,
      polyline: '',
      start: _t(from),
      end: _t(to),
    );

Itinerary _itin(int legs) => Itinerary(
      start: _t(0),
      end: _t(5 * legs),
      durationSec: 300.0 * legs,
      transfers: 0,
      walkM: 222.0 * legs,
      legs: [for (var i = 0; i < legs; i++) _walk(i * 5, (i + 1) * 5)],
    );

const _request = PlanRequest(
  origin: Place(name: '출발', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '도착', address: '', lat: 37.502, lon: 127.0),
);

GuideSession _session(int legs) =>
    GuideSession(api: _Api(), request: _request, itinerary: _itin(legs), clock: () => _base);

void main() {
  tearDown(ActiveGuide.instance.clear);

  // clear 와 set 이 한 프레임 안에서 일어나면 중간 빈 위젯이 그려지지 않아 요약 상태가 그대로 남는다.
  testWidgets('안내를 갈아타면 요약이 새 안내를 따라간다', (tester) async {
    ActiveGuide.instance.set(_session(2));
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: CurrentGuideTab())));
    await tester.pump();
    expect(find.textContaining('구간 1/2'), findsOneWidget);

    final b = _session(3);
    ActiveGuide.instance.clear();
    ActiveGuide.instance.set(b); // 같은 프레임 — 상세 화면의 "새로 시작" 흐름과 같다
    await tester.pump();
    expect(find.textContaining('구간 1/3'), findsOneWidget);

    b.nextLeg();
    await tester.pump();
    expect(find.textContaining('구간 2/3'), findsOneWidget);
  });

}
