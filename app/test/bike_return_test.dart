import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/bike_return.dart';

// 위도 0.00001도 ≈ 1.1m. 계획 대여소·경로선에서 충분히 떨어진 자리에서 북쪽으로 걷는다.
void main() {
  final t0 = DateTime(2026, 9, 30, 9);

  bool feed(BikeReturnDetector d, int sec, {double lat = 37.5, double speed = 1.2, double stationM = 400,
      double routeM = 180, double acc = 8}) =>
      d.update(
        lat: lat,
        lon: 127.0,
        stationM: stationM,
        routeM: routeM,
        speedMps: speed,
        accuracyM: acc,
        at: t0.add(Duration(seconds: sec)),
      );

  /// 2초마다 1.2m/s 로 북쪽으로 걷는 표본을 sec 초까지 넣는다.
  bool walk(BikeReturnDetector d, int untilSec, {double speed = 1.2, double stationM = 400, double routeM = 180}) {
    var fired = false;
    for (var s = 0; s <= untilSec; s += 2) {
      fired |= feed(d, s, lat: 37.5 + s * 1.2 / 111195, speed: speed, stationM: stationM, routeM: routeM);
    }
    return fired;
  }

  test('대여소·경로선에서 떨어져 90초 넘게 걷는 속도로 60m 넘게 움직이면 한 번 참', () {
    final d = BikeReturnDetector();
    expect(walk(d, 88), isFalse);
    expect(feed(d, 90, lat: 37.5 + 90 * 1.2 / 111195), isTrue);
    expect(feed(d, 92, lat: 37.5 + 92 * 1.2 / 111195), isFalse); // 창을 새로 시작한다
  });

  test('계획 대여소 150m 안, 경로선 60m 안, 자전거 속도면 창을 지운다', () {
    expect(walk(BikeReturnDetector(), 120, stationM: 120), isFalse);
    expect(walk(BikeReturnDetector(), 120, routeM: 40), isFalse);
    expect(walk(BikeReturnDetector(), 120, speed: 4.0), isFalse);
    final d = BikeReturnDetector();
    walk(d, 80);
    feed(d, 82, speed: 4.0); // 한 번 자전거 속도가 나오면 처음부터
    expect(walk(d, 88), isFalse);
  });

  test('제자리(60m 미만)면 참이 되지 않고, 걷는 속도를 넘는 이동이면 창을 지운다', () {
    final still = BikeReturnDetector();
    var fired = false;
    for (var s = 0; s <= 120; s += 2) {
      fired |= feed(still, s, speed: 0); // 속도 미상·제자리
    }
    expect(fired, isFalse);

    final jump = BikeReturnDetector();
    feed(jump, 0);
    expect(feed(jump, 2, lat: 37.5 + 100 / 111195, speed: 0), isFalse); // 2초에 100m
    expect(walk(jump, 88), isFalse); // 창이 새로 시작됐으므로 88초로는 아직
  });

  test('오차 큰 표본은 창을 건드리지 않는다', () {
    final d = BikeReturnDetector();
    walk(d, 60);
    expect(feed(d, 62, stationM: 10, acc: 90), isFalse);
    expect(feed(d, 90, lat: 37.5 + 90 * 1.2 / 111195), isTrue);
  });
}
