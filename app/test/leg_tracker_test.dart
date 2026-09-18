import 'package:flutter_test/flutter_test.dart';
import 'package:latlong2/latlong.dart';

import 'package:seoul_route/guide/leg_tracker.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/util/polyline.dart';

import 'support/polyline_encode.dart';

Leg leg(String mode, double toLat, double toLon, {bool rented = false, String polyline = ''}) => Leg(
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
      transitLeg: mode == 'BUS' || mode == 'SUBWAY',
      polyline: polyline,
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

  test('경로선을 한 번 풀어 두고 화면이 같이 쓴다', () {
    final line = encodePolyline([const LatLng(37.5, 127.0), const LatLng(37.502, 127.0)]);
    final t = LegTracker([leg('WALK', 37.502, 127.0, polyline: line), leg('WALK', 37.51, 127.0)]);
    expect(t.points[0], decodePolyline(line));
    expect(t.points[1], [const LatLng(37.5, 127.0), const LatLng(37.51, 127.0)]); // 경로선이 없으면 양끝 직선
  });

  group('다음 구간 경로선 위로 들어오면 넘긴다(지하 승강장은 끝점 40m 안에 못 들어간다)', () {
    // 지하철 구간 끝 = 승강장 (37.500, 127.000). 다음 도보는 거기서 북쪽으로 약 222m.
    final walkLine = encodePolyline([const LatLng(37.5, 127.0), const LatLng(37.502, 127.0)]);
    List<Leg> subwayThenWalk() =>
        [leg('SUBWAY', 37.5, 127.0), leg('WALK', 37.502, 127.0, polyline: walkLine), leg('WALK', 37.51, 127.0)];

    test('연속 두 번 맞아야 넘어간다', () {
      final t = LegTracker(subwayThenWalk());
      expect(t.update(37.5006, 127.0, accuracyM: 8), isFalse); // 경로선 위 67m — 한 번으로는 안 넘긴다
      expect(t.index, 0);
      expect(t.update(37.5006, 127.0, accuracyM: 8), isTrue);
      expect(t.index, 1);
    });

    test('정확도가 나쁜 표본은 세지 않는다', () {
      final t = LegTracker(subwayThenWalk());
      for (var i = 0; i < 3; i++) {
        expect(t.update(37.5006, 127.0, accuracyM: 80), isFalse);
      }
      expect(t.index, 0);
    });

    test('다음 경로선을 스치기만 하면(초입) 넘기지 않는다', () {
      // 현재 도보 경로선은 북쪽으로 뻗은 세로선, 다음 구간 경로선은 그 아래를 지나는 가로선이다.
      // 가로선 위에 있어도 얼마 가지 않았으면 아직 그 구간을 타기 시작한 것이 아니다. 경도 0.0001도 ≈ 8.8m.
      final cross = [
        leg('WALK', 37.5027, 127.0018,
            polyline: encodePolyline([const LatLng(37.5, 127.0018), const LatLng(37.5027, 127.0018)])),
        leg('WALK', 37.5, 127.0034,
            polyline: encodePolyline([const LatLng(37.5, 127.0), const LatLng(37.5, 127.0034)])),
        leg('WALK', 37.51, 127.0),
      ];
      final early = LegTracker(cross);
      for (var i = 0; i < 3; i++) {
        early.update(37.5, 127.0003, accuracyM: 8); // 가로선 위 27m — 아직 초입
      }
      expect(early.index, 0);

      final along = LegTracker(cross);
      expect(along.update(37.5, 127.0007, accuracyM: 8), isFalse); // 62m 진행, 한 번으로는 안 넘긴다
      expect(along.update(37.5, 127.0007, accuracyM: 8), isTrue);
      expect(along.index, 1);
    });

    test('지상 구간끼리는 현재 경로선에서 벗어났을 때만 넘어간다', () {
      // 도보 경로선은 (37.5,127.0)→(37.502,127.0), 다음 자전거 경로선은 그 동쪽 100m 에 나란히 놓인다.
      const eastLon = 127.00113; // 위도 37.5 에서 약 100m
      final bikeLine =
          encodePolyline([const LatLng(37.5, eastLon), const LatLng(37.502, eastLon)]);
      final near = LegTracker([
        leg('WALK', 37.502, 127.0, polyline: encodePolyline([const LatLng(37.5, 127.0), const LatLng(37.502, 127.0)])),
        leg('BICYCLE', 37.502, eastLon, rented: true, polyline: bikeLine),
        leg('WALK', 37.51, 127.0),
      ]);
      for (var i = 0; i < 3; i++) {
        near.update(37.5006, eastLon, accuracyM: 8); // 자전거 경로선 위지만 도보 경로선에서 100m
      }
      expect(near.index, 1);

      final parallel = LegTracker([
        leg('WALK', 37.502, 127.0, polyline: encodePolyline([const LatLng(37.5, 127.0), const LatLng(37.502, 127.0)])),
        leg('BICYCLE', 37.502, 127.0003, rented: true,
            polyline: encodePolyline([const LatLng(37.5, 127.0003), const LatLng(37.502, 127.0003)])),
        leg('WALK', 37.51, 127.0),
      ]);
      for (var i = 0; i < 3; i++) {
        parallel.update(37.5006, 127.0003, accuracyM: 8); // 두 경로선이 27m 옆에 나란히 — 넘기지 않는다
      }
      expect(parallel.index, 0);
    });
  });
}
