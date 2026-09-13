import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/leg_tracker.dart';
import 'package:seoul_route/models/itinerary.dart';

Leg leg(String mode, double toLat, double toLon, {bool rented = false}) => Leg(
      mode: mode,
      durationSec: 60,
      distanceM: 100,
      fromName: 'a',
      toName: 'b',
      fromLat: 37.5,
      fromLon: 127.0,
      toLat: toLat,
      toLon: toLon,
      route: '',
      rentedBike: rented,
      transitLeg: mode == 'BUS',
      polyline: '',
      start: '',
      end: '',
    );

void main() {
  // 위도 1도 ≈ 111,195m. 구간 1 끝 (37.501, 127.0), 구간 2 끝 (37.502, 127.0).
  final legs = [leg('WALK', 37.501, 127.0), leg('BICYCLE', 37.502, 127.0, rented: true), leg('BUS', 37.51, 127.0)];

  test('끝점 40m 안에 들어오면 다음 구간, 밖이면 그대로', () {
    final t = LegTracker(legs);
    expect(t.mode, 'walk');
    expect(t.update(37.5005, 127.0), isFalse); // 끝점에서 56m
    expect(t.index, 0);
    expect(t.update(37.50070, 127.0), isTrue); // 33m
    expect(t.index, 1);
    expect(t.mode, 'bicycle');
  });

  test('마지막 구간 끝에 닿아도 index 는 머문다, 손으로 앞뒤 이동', () {
    final t = LegTracker(legs);
    t.next();
    t.next();
    expect(t.index, 2);
    expect(t.mode, 'transit');
    expect(t.isLast, isTrue);
    expect(t.update(37.51, 127.0), isFalse);
    expect(t.index, 2);
    t.next();
    expect(t.index, 2);
    t.prev();
    expect(t.index, 1);
    t.prev();
    t.prev();
    expect(t.index, 0);
  });

  test('거리 계산: 위도 0.001도 ≈ 111m', () {
    expect(LegTracker.distanceM(37.5, 127.0, 37.501, 127.0), closeTo(111.2, 0.5));
  });
}
