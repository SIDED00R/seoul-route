import 'package:flutter_test/flutter_test.dart';
import 'package:latlong2/latlong.dart';

import 'package:seoul_route/guide/geo.dart';
import 'package:seoul_route/guide/leg_tracker.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/util/polyline.dart';

import 'support/polyline_encode.dart';

Leg leg(String mode, double toLat, double toLon,
        {bool rented = false,
        String polyline = '',
        String start = '',
        String end = '',
        bool inStation = false,
        List<String> nextDepartures = const []}) =>
    Leg(
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
      start: start,
      end: end,
      inStation: inStation,
      nextDepartures: nextDepartures,
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
    expect(distanceM(37.5, 127.0, 37.501, 127.0), closeTo(111.2, 0.5));
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

  group('지하에서는 시간표로 구간을 넘긴다(위치가 잡히지 않는다)', () {
    // 09:00 도보 → 09:05~09:20 2호선 → 09:20~09:23 역 안 환승 통로 → 09:25~09:28 4호선 → 09:28~ 도보
    DateTime at(int h, int m, [int s = 0]) => DateTime(2026, 9, 18, h, m, s);
    String ts(int h, int m) => at(h, m).toIso8601String();
    List<Leg> ride() => [
          leg('WALK', 37.501, 127.0, start: ts(9, 0), end: ts(9, 5)),
          leg('SUBWAY', 37.52, 127.0, start: ts(9, 5), end: ts(9, 20), nextDepartures: [ts(9, 9), ts(9, 13)]),
          leg('WALK', 37.5201, 127.0, start: ts(9, 20), end: ts(9, 23), inStation: true),
          leg('SUBWAY', 37.53, 127.0, start: ts(9, 25), end: ts(9, 28)),
          leg('WALK', 37.531, 127.0, start: ts(9, 28), end: ts(9, 33)),
        ];

    test('위치가 끊긴 채 도착 시각이 지나면 다음 구간으로 넘어간다', () {
      final t = LegTracker(ride(), now: at(9, 0));
      t.next(now: at(9, 4)); // 계획보다 일찍 승강장에 옴 → 계획한 09:05 차를 탄다
      expect(t.shift, Duration.zero);
      expect(t.tick(at(9, 19, 59)), isFalse);
      expect(t.tick(at(9, 20)), isTrue); // 환승 통로
      expect(t.index, 2);
      expect(t.tick(at(9, 23)), isTrue); // 4호선
      expect(t.index, 3);
      expect(t.tick(at(9, 27)), isFalse);
      expect(t.tick(at(9, 28)), isTrue);
      expect(t.index, 4);
      expect(t.tick(at(10, 0)), isFalse); // 마지막 구간에 머문다
    });

    test('정확도가 나쁜 표본만 와도 같은 판정을 한다', () {
      final t = LegTracker(ride(), now: at(9, 0));
      t.next(now: at(9, 4));
      expect(t.update(37.5, 127.0, accuracyM: 90, now: at(9, 19)), isFalse);
      expect(t.update(37.5, 127.0, accuracyM: 90, now: at(9, 20, 5)), isTrue);
      expect(t.index, 2);
    });

    test('믿을 만한 위치가 방금까지 있었으면 시간표로 넘기지 않는다(지상 구간)', () {
      final t = LegTracker(ride(), now: at(9, 0));
      t.next(now: at(9, 4));
      // 끝점에서도 다음 경로선에서도 먼 곳의 좋은 표본
      expect(t.update(37.51, 127.01, accuracyM: 8, now: at(9, 20, 5)), isFalse);
      expect(t.tick(at(9, 20, 10)), isFalse);
      expect(t.tick(at(9, 20, 30)), isTrue); // 좋은 위치가 20초 넘게 끊기면 지하로 본다
    });

    test('도보·자전거 구간은 시간이 지나도 시간표로 넘기지 않는다', () {
      final t = LegTracker(ride(), now: at(9, 0));
      expect(t.tick(at(9, 30)), isFalse);
      expect(t.index, 0);
    });

    test('승강장에 늦게 닿으면(자동 넘김) 다음 차 시각만큼 뒤 구간을 민다', () {
      final t = LegTracker(ride(), now: at(9, 0));
      expect(t.update(37.501, 127.0, accuracyM: 8, now: at(9, 7)), isTrue); // 09:05 차를 놓침 → 09:09 차
      expect(t.shift, const Duration(minutes: 4));
      expect(t.tick(at(9, 20, 30)), isFalse);
      expect(t.tick(at(9, 24)), isTrue); // 09:20 + 4분
      expect(t.index, 2);
    });

    test('다음 차 시각을 모르면 늦은 만큼 민다', () {
      final legs = ride()..[1] = leg('SUBWAY', 37.52, 127.0, start: ts(9, 5), end: ts(9, 20));
      final t = LegTracker(legs, now: at(9, 0));
      expect(t.update(37.501, 127.0, accuracyM: 8, now: at(9, 8)), isTrue);
      expect(t.shift, const Duration(minutes: 3));
    });

    test('안내를 늦게 시작하면 첫 구간부터 그만큼 민다', () {
      expect(LegTracker(ride(), now: at(9, 3)).shift, const Duration(minutes: 3));
      expect(LegTracker(ride(), now: at(8, 58)).shift, Duration.zero);
    });

    test('손으로 넘기면 이미 탄 것으로 보고 가장 최근에 떠난 차를 기준으로 한다', () {
      final onPlanned = LegTracker(ride(), now: at(9, 0));
      onPlanned.next(now: at(9, 8)); // 계획한 09:05 차에 탄 채 화면이 늦어 누름
      expect(onPlanned.shift, Duration.zero);
      final onLater = LegTracker(ride(), now: at(9, 0));
      onLater.next(now: at(9, 10)); // 09:09 차
      expect(onLater.shift, const Duration(minutes: 4));
    });

    test('손으로 되돌린 구간은 시간표 인계가 곧바로 다시 넘기지 않는다', () {
      final t = LegTracker(ride(), now: at(9, 0));
      t.next(now: at(9, 4));
      expect(t.tick(at(9, 20)), isTrue); // 열차가 늦었는데 시간표로 환승 통로까지 넘어감
      t.prev(now: at(9, 21));
      expect(t.index, 1);
      expect(t.tick(at(9, 21, 10)), isFalse);
      expect(t.tick(at(9, 27)), isFalse); // 가장 최근 차(09:13) 기준 예상 도착 09:28
      expect(t.tick(at(9, 28)), isTrue);
    });

    test('손으로 되돌렸을 때 예상 종료가 이미 지났으면 늦은 만큼 밀어 곧바로 다시 넘기지 않는다', () {
      final t = LegTracker(ride(), now: at(9, 0));
      t.next(now: at(9, 4));
      expect(t.tick(at(9, 20)), isTrue);
      t.prev(now: at(9, 29)); // 가장 최근 차(09:13) 기준 예상 도착 09:28 이 이미 지났다
      expect(t.shift, const Duration(minutes: 24));
      expect(t.tick(at(9, 29, 5)), isFalse);
      expect(t.index, 1);
    });

    test('버스 구간도 위치가 끊기면 시간표로 넘긴다', () {
      final t = LegTracker([
        leg('BUS', 37.52, 127.0, start: ts(9, 5), end: ts(9, 20)),
        leg('WALK', 37.521, 127.0, start: ts(9, 20), end: ts(9, 25)),
      ], now: at(9, 4));
      expect(t.tick(at(9, 19)), isFalse);
      expect(t.tick(at(9, 20)), isTrue);
    });
  });

  test('지상에 나와 뒤쪽 도보 구간 경로선 위에 있으면 그 구간으로 건너뛴다', () {
    // 지하에서 구간이 밀려 첫 지하철에 머문 채, 마지막 도보(37.53 에서 북쪽으로) 경로선 위 67m 지점에 나타난다.
    final last = encodePolyline([const LatLng(37.53, 127.0), const LatLng(37.532, 127.0)]);
    final t = LegTracker([
      leg('SUBWAY', 37.52, 127.0),
      leg('WALK', 37.5201, 127.0, inStation: true),
      leg('SUBWAY', 37.53, 127.0),
      leg('WALK', 37.532, 127.0, polyline: last),
    ]);
    expect(t.update(37.5306, 127.0, accuracyM: 8), isFalse);
    expect(t.update(37.5306, 127.0, accuracyM: 8), isTrue);
    expect(t.index, 3);
  });

  test('지상 도보 중에는 두 구간 뒤 도보 경로선 위에 있어도 건너뛰지 않는다', () {
    // 첫 도보 경로선에서 동쪽으로 약 150m 떨어진 나란한 길이 마지막 도보 구간이다.
    const eastLon = 127.0017;
    final t = LegTracker([
      leg('WALK', 37.502, 127.0,
          polyline: encodePolyline([const LatLng(37.5, 127.0), const LatLng(37.502, 127.0)])),
      leg('SUBWAY', 37.52, 127.0),
      leg('WALK', 37.502, eastLon,
          polyline: encodePolyline([const LatLng(37.5, eastLon), const LatLng(37.502, eastLon)])),
    ]);
    for (var i = 0; i < 3; i++) {
      t.update(37.5006, eastLon, accuracyM: 8);
    }
    expect(t.index, 0);
  });
}
