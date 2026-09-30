import 'package:flutter_test/flutter_test.dart';
import 'package:latlong2/latlong.dart';

import 'package:seoul_route/guide/leg_tracker.dart';
import 'package:seoul_route/models/itinerary.dart';

import 'support/polyline_encode.dart';

// 09:00~09:10 2호선(37.50 → 37.52) → 09:10~09:13 도보(37.52 → 37.5215, 대여소) → 09:13~ 따릉이(→ 37.53).
DateTime at(int m, [int s = 0]) => DateTime(2026, 9, 28, 9, m, s);

Leg leg(String mode, double fromLat, double toLat, int startMin, int endMin, {bool rented = false}) => Leg(
      mode: mode,
      durationSec: (endMin - startMin) * 60.0,
      distanceM: 100,
      fromName: 'a',
      toName: 'b',
      fromLat: fromLat,
      fromLon: 127.0,
      toLat: toLat,
      toLon: 127.0,
      route: '',
      rentedBike: rented,
      transitLeg: mode == 'BUS' || mode == 'SUBWAY',
      polyline: encodePolyline([LatLng(fromLat, 127.0), LatLng(toLat, 127.0)]),
      start: at(startMin).toIso8601String(),
      end: at(endMin).toIso8601String(),
    );

List<Leg> subwayThenBike() => [
      leg('SUBWAY', 37.50, 37.52, 0, 10),
      leg('WALK', 37.52, 37.5215, 10, 13),
      leg('BICYCLE', 37.5215, 37.53, 13, 25, rented: true),
    ];

void main() {
  test('탈것 안이면 지하에서 시간표 넘김을 미루고, 내려서 걸으면 넘긴다', () {
    final t = LegTracker(subwayThenBike(), now: at(0));
    expect(t.tick(at(10, 30), activity: 'vehicle'), isFalse);
    expect(t.index, 0);
    expect(t.tick(at(10, 40), activity: 'walk'), isTrue);
    expect(t.index, 1);
  });

  test('활동을 모르면(권한 없음) 지금처럼 시간표로 넘긴다', () {
    final t = LegTracker(subwayThenBike(), now: at(0));
    expect(t.tick(at(10, 30)), isTrue);
    expect(t.index, 1);
  });

  test('탈것 안이면 역 도착 반경·다음 경로선 인계로도 도보로 넘기지 않는다', () {
    final t = LegTracker(subwayThenBike(), now: at(0));
    expect(t.update(37.5199, 127.0, accuracyM: 10, now: at(9, 50), activity: 'vehicle'), isFalse); // 끝점 11m
    expect(t.update(37.5210, 127.0, accuracyM: 10, now: at(9, 55), activity: 'vehicle'), isFalse); // 도보 경로선 111m
    expect(t.update(37.5210, 127.0, accuracyM: 10, now: at(10, 0), activity: 'vehicle'), isFalse);
    expect(t.index, 0);
    expect(t.update(37.5199, 127.0, accuracyM: 10, now: at(10, 5), activity: 'walk'), isTrue);
    expect(t.index, 1);
  });

  test('내려서 걷는 구간도 탈것 안이면 대여소 옆 위치 하나로 따릉이로 넘기지 않는다', () {
    final t = LegTracker(subwayThenBike(), now: at(0));
    t.next(now: at(10)); // 시간표 넘김 등으로 이미 도보 구간
    expect(t.update(37.5214, 127.0, accuracyM: 49, now: at(10, 10), activity: 'vehicle'), isFalse); // 끝점 11m
    expect(t.index, 1);
    expect(t.update(37.5214, 127.0, accuracyM: 10, now: at(11), activity: 'walk'), isTrue);
    expect(t.index, 2);
  });

  test('다음이 대중교통인 환승 도보로는 탈것 안이어도 넘긴다', () {
    final t = LegTracker([
      leg('SUBWAY', 37.50, 37.51, 0, 10),
      leg('WALK', 37.51, 37.5101, 10, 12),
      leg('SUBWAY', 37.5101, 37.52, 13, 20),
    ], now: at(0));
    expect(t.update(37.5099, 127.0, accuracyM: 10, now: at(10), activity: 'vehicle'), isTrue);
    expect(t.index, 1);
  });

  test('대중교통끼리 이어지는 환승은 탈것 안이어도 넘긴다', () {
    final t = LegTracker([leg('BUS', 37.50, 37.51, 0, 10), leg('BUS', 37.51, 37.52, 11, 20)], now: at(0));
    expect(t.update(37.5099, 127.0, accuracyM: 10, now: at(10), activity: 'vehicle'), isTrue);
    expect(t.index, 1);
  });

  test('걷다가 따릉이로 넘어가는 보통 구간은 활동과 무관하다(자전거를 탈것으로 잘못 판정해도)', () {
    final t = LegTracker([
      leg('WALK', 37.50, 37.501, 0, 2),
      leg('BICYCLE', 37.501, 37.51, 2, 10, rented: true),
      leg('WALK', 37.51, 37.511, 10, 12),
    ], now: at(0));
    expect(t.update(37.5009, 127.0, accuracyM: 10, now: at(2), activity: 'vehicle'), isTrue);
    expect(t.update(37.5099, 127.0, accuracyM: 10, now: at(10), activity: 'vehicle'), isTrue);
    expect(t.index, 2);
  });
}
