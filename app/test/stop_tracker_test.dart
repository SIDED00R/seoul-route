import 'package:flutter_test/flutter_test.dart';
import 'package:latlong2/latlong.dart';

import 'package:seoul_route/guide/stop_tracker.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/leg_detail.dart';

// 남쪽 (37.5) 에서 북쪽 (37.504) 으로 가는 지하철 구간. 중간 정차 3곳(111m 간격), 09:00 출발.
const start = '2026-09-18T09:00:00+09:00';
final line = [const LatLng(37.5, 127.0), const LatLng(37.504, 127.0)];
final leg = Leg(
  mode: 'SUBWAY',
  durationSec: 480,
  distanceM: 445,
  fromName: '출발역',
  toName: '하차역',
  fromLat: 37.5,
  fromLon: 127.0,
  toLat: 37.504,
  toLon: 127.0,
  route: '2호선',
  rentedBike: false,
  transitLeg: true,
  polyline: '',
  start: start,
  end: '2026-09-18T09:08:00+09:00',
  headsign: '성수',
  stops: const [
    TransitStop(name: '역1', lat: 37.501, lon: 127.0, offsetSec: 120),
    TransitStop(name: '역2', lat: 37.502, lon: 127.0, offsetSec: 240),
    TransitStop(name: '역3', lat: 37.503, lon: 127.0, offsetSec: 360),
  ],
);
DateTime at(int sec) => DateTime.parse(start).add(Duration(seconds: sec));

void main() {
  test('지상에서는 위치로 남은 정거장을 센다', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(37.5, 127.0, 8, at(0)), 4); // 정차 3 + 하차역
    expect(t.nextStopName(), '역1');
    expect(t.remaining(37.5015, 127.0, 8, at(60)), 3);
    expect(t.nextStopName(), '역2');
    expect(t.remaining(37.5035, 127.0, 8, at(300)), 1);
    expect(t.nextStopName(), isNull); // 다음이 하차역
  });

  test('위치가 없거나 정확도가 나쁘면 시간표로 센다(지하 구간)', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(null, null, 0, at(10)), 4);
    expect(t.remaining(37.5, 127.0, 90, at(250)), 2); // 역1·역2 통과, 역3 과 하차역만 남음
    expect(t.nextStopName(), '역3');
  });

  test('한 번 줄어든 값은 다시 늘지 않는다', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(37.5035, 127.0, 8, at(300)), 1);
    expect(t.remaining(37.5, 127.0, 8, at(310)), 1); // 위치가 뒤로 튀어도 그대로
  });

  test('중간 정차가 없으면 하차역 하나만 남는다', () {
    final bare = StopTracker(
      Leg(
        mode: 'BUS',
        durationSec: 300,
        distanceM: 1000,
        fromName: 'a',
        toName: 'b',
        fromLat: 37.5,
        fromLon: 127.0,
        toLat: 37.504,
        toLon: 127.0,
        route: '402',
        rentedBike: false,
        transitLeg: true,
        polyline: '',
        start: start,
        end: '2026-09-18T09:05:00+09:00',
      ),
      line,
    );
    expect(bare.remaining(37.5, 127.0, 8, at(0)), 1);
    expect(bare.nextStopName(), isNull);
  });
}
