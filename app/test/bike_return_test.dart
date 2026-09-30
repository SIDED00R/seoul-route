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

  /// fromSec 부터 untilSec 초까지 2초마다 북쪽으로 실제 mps 로 움직이는 표본을 넣는다(위치는 0초 기준). speed 는
  /// 보고 속도.
  bool walk(BikeReturnDetector d, int untilSec,
      {int fromSec = 0, double mps = 1.2, double speed = 1.2, double stationM = 400, double routeM = 180}) {
    var fired = false;
    for (var s = fromSec; s <= untilSec; s += 2) {
      fired |= feed(d, s, lat: 37.5 + s * mps / 111195, speed: speed, stationM: stationM, routeM: routeM);
    }
    return fired;
  }

  test('대여소·경로선에서 떨어져 90초 넘게 걷는 속도로 60m 넘게 움직이면 한 번 참', () {
    final d = BikeReturnDetector();
    expect(walk(d, 88), isFalse);
    expect(feed(d, 90, lat: 37.5 + 90 * 1.2 / 111195), isTrue);
    expect(feed(d, 92, lat: 37.5 + 92 * 1.2 / 111195), isFalse); // 창을 새로 시작한다
  });

  test('계획 대여소 150m 안, 경로선 60m 안, 자전거 속도가 5표본 이어지면 창을 지운다', () {
    expect(walk(BikeReturnDetector(), 120, stationM: 120), isFalse);
    expect(walk(BikeReturnDetector(), 120, routeM: 40), isFalse);
    expect(walk(BikeReturnDetector(), 120, mps: 4.0, speed: 4.0), isFalse);
    final d = BikeReturnDetector();
    walk(d, 80);
    walk(d, 88, fromSec: 82, speed: 2.0); // 4표본은 견딘다
    expect(walk(d, 92, fromSec: 90), isTrue);
    final e = BikeReturnDetector();
    walk(e, 80);
    expect(walk(e, 90, fromSec: 82, speed: 2.0), isFalse); // 5표본째에 처음부터
    expect(walk(e, 170, fromSec: 92), isFalse);
    expect(walk(e, 182, fromSec: 172), isTrue);
  });

  test('걷는 표본의 속도가 한 번 크게 튀어도 창을 지우지 않는다', () {
    final d = BikeReturnDetector();
    walk(d, 80);
    feed(d, 82, lat: 37.5 + 82 * 1.2 / 111195, speed: 3.8);
    expect(walk(d, 90, fromSec: 84), isTrue);
  });

  test('2m/s 안팎으로 계속 타거나 신호에 서며 타면 참이 되지 않는다', () {
    expect(walk(BikeReturnDetector(), 180, mps: 2.0, speed: 2.0), isFalse);
    expect(walk(BikeReturnDetector(), 180, mps: 2.0, speed: 0), isFalse); // 속도 미상이어도 평균으로
    // 2.4m/s 로 40초 타고 50초 서기를 반복(평균 1.07m/s): 타는 동안의 속도 표본이 이어져 창이 지워진다
    final d = BikeReturnDetector();
    var fired = false;
    var lat = 37.5;
    for (var s = 0; s <= 270; s += 2) {
      final riding = s % 90 < 40;
      if (riding) lat += 2 * 2.4 / 111195;
      fired |= feed(d, s, lat: lat, speed: riding ? 2.4 : 0.3);
    }
    expect(fired, isFalse);
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
