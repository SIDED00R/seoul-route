import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:latlong2/latlong.dart';

import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/plan_screen.dart';
import 'package:seoul_route/settings/settings_store.dart';
import 'package:seoul_route/util/polyline.dart';

void main() {
  test('polyline decode: Google 문서 예제', () {
    // https://developers.google.com/maps/documentation/utilities/polylinealgorithm 의 예제
    final pts = decodePolyline('_p~iF~ps|U_ulLnnqC_mqNvxq`@');
    expect(pts.length, 3);
    expect(pts[0].latitude, closeTo(38.5, 1e-5));
    expect(pts[0].longitude, closeTo(-120.2, 1e-5));
    expect(pts[2], LatLng(43.252, -126.453));
  });

  test('PlanRequest.toJson: 전 구간 any 면 segment_modes 를 보내지 않는다', () {
    const o = Place(name: 'a', address: '', lat: 37.55, lon: 126.97);
    const d = Place(name: 'b', address: '', lat: 37.50, lon: 127.03);
    final j = PlanRequest(origin: o, destination: d, segmentModes: const [SegmentMode.any]).toJson();
    expect(j.containsKey('segment_modes'), isFalse);
    expect(j.containsKey('via'), isFalse);
    final j2 = PlanRequest(
      origin: o, destination: d, via: const [o], segmentModes: const [SegmentMode.walk, SegmentMode.bike],
    ).toJson();
    expect(j2['segment_modes'], ['walk', 'bike']);
    expect((j2['via'] as List).length, 1);
  });

  test('PlanResult.fromJson: 서버 응답 형식', () {
    final r = PlanResult.fromJson({
      'itineraries': [
        {
          'start': 's', 'end': 'e', 'duration_sec': 1830, 'transfers': 1, 'walk_distance_m': 512.3,
          'legs': [
            {'mode': 'WALK', 'duration_sec': 120, 'distance_m': 150, 'from_lat': 37.55, 'from_lon': 126.97,
             'to_lat': 37.551, 'to_lon': 126.971, 'transit_leg': false},
            {'mode': 'BICYCLE', 'duration_sec': 900, 'distance_m': 3000, 'from_lat': 37.551, 'from_lon': 126.971,
             'to_lat': 37.56, 'to_lon': 126.98, 'rented_bike': true, 'transit_leg': false, 'polyline': '_p~iF~ps|U'},
          ],
        }
      ],
      'walk_speed': 1.2,
      'note': 'n',
    });
    expect(r.itineraries.single.minutes, 31);
    expect(r.itineraries.single.legs[1].label, '따릉이');
    expect(r.reason, isNull);
    final empty = PlanResult.fromJson({'itineraries': [], 'reason': '경로 없음: 구간 1'});
    expect(empty.itineraries, isEmpty);
    expect(empty.reason, '경로 없음: 구간 1');
  });

  testWidgets('설정이 비어 있으면 안내 카드가 보이고 탐색 버튼은 비활성', (tester) async {
    await tester.pumpWidget(const MaterialApp(
      home: PlanScreen(settings: Settings(baseUrl: Settings.defaultBaseUrl, token: '')),
    ));
    expect(find.text('서버 주소와 토큰을 먼저 설정하세요'), findsOneWidget);
    final btn = tester.widget<FilledButton>(find.byType(FilledButton));
    expect(btn.onPressed, isNull);
  });
}
