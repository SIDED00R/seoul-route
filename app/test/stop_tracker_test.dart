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

  test('다음 차를 탔으면 정차 시각도 그만큼 늦다', () {
    final t = StopTracker(leg, line, shift: const Duration(minutes: 4));
    expect(t.remaining(null, null, 0, at(250)), 4); // 밀지 않았다면 2 였을 시각
    expect(t.remaining(null, null, 0, at(250 + 240)), 2);
  });

  // 계획한 차를 놓치고 늦은 차를 타면, 구간에 들어올 때 한 번 잰 shift 로는 그 지연을 알 수 없다(승강장에 닿기 전에
  // 구간이 넘어가면 shift 는 0 이다). 승강장에서 계획 시각이 지나도록 기다린 것을 위치로 읽어 시간표를 민다.
  test('계획보다 늦은 열차: 기다린 만큼 시간표를 민다', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(37.5, 127.0, 8, at(0)), 4);
    // 계획 출발(0초)이 지나도록 승강장에 그대로 있다 — 열차는 적어도 240초 늦다.
    expect(t.remaining(37.5, 127.0, 8, at(240)), 4);
    // 위치가 끊긴 채 계획 하차 시각(480초)이 지나도, 민 시간표로는 아직 역1 도 지나지 않았다.
    expect(t.remaining(null, null, 0, at(300)), 4);
    expect(t.remaining(null, null, 0, at(470)), 3);
    expect(t.nextStopName(), '역2');
    expect(t.remaining(null, null, 0, at(721)), 1); // 민 시간표로 하차역 직전
    expect(t.nextStopName(), isNull);
  });

  // 정시 열차가 지하로 들어가면 위치가 승강장 표본 하나뿐이다. 그때도 시간표가 계속 줄어 하차 안내가 나가야 한다.
  test('정시 열차: 승강장 표본 하나 뒤 위치가 끊겨도 시간표가 센다', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(37.5, 127.0, 8, at(0)), 4);
    expect(t.remaining(null, null, 0, at(121)), 3);
    expect(t.remaining(null, null, 0, at(241)), 2);
    expect(t.remaining(null, null, 0, at(361)), 1);
    expect(t.nextStopName(), isNull);
  });

  // 정차 사이를 계획대로 달리는 것은 지연이 아니다. 진행 거리로 계획 시각을 나눠 재지 않고 직전 정차 시각과만
  // 견주면 정상 주행 시간이 지연으로 쌓여, 위치가 끊긴 뒤 남은 정거장 수가 한 정거장 늦게 줄어든다.
  test('정시 주행 중의 위치는 지연으로 쌓이지 않는다', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(37.5005, 127.0, 8, at(60)), 4); // 출발역과 역1 사이 절반, 계획대로
    expect(t.remaining(null, null, 0, at(121)), 3); // 역1 계획 시각이 지나면 그대로 줄어든다
  });

  // 뒤로 크게 튄 표본을 "지금 여기" 로 읽으면 지연이 크게 잡혀 굳고, 이번에는 하차 안내가 되레 늦게 나간다.
  test('뒤로 튄 표본은 지연으로 재지 않는다', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(37.5008, 127.0, 8, at(96)), 3); // 역1 을 20여 m 앞둔 지점 — 통과로 센다
    expect(t.remaining(37.5, 127.0, 8, at(240)), 3); // 출발점으로 튄 표본
    expect(t.remaining(null, null, 0, at(361)), 1); // 지연이 잡히지 않아 시간표가 그대로 센다
  });

  // 지연은 늦은 쪽으로만 키운다. 계획보다 앞선 지점을 보고 줄이면 하차 안내가 되레 일찍 나간다.
  test('지연은 줄어들지 않는다', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(37.5, 127.0, 8, at(240)), 4); // 승강장에서 240초 기다렸다
    expect(t.remaining(37.5005, 127.0, 8, at(250)), 4); // 계획(60초)보다 190초 늦은 지점 — 240초를 줄이지 않는다
    expect(t.remaining(null, null, 0, at(330)), 4);
  });

  // 승강장에서 탈 칸을 찾아 진행 반대쪽으로 걷는 것은 "뒤로 튄 표본" 이 아니다. 그동안 기다린 시간도 읽어야 한다.
  test('승강장에서 진행 반대쪽으로 걸어도 기다린 시간을 읽는다', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(37.50018, 127.0, 8, at(0)), 4); // 출발역에서 20m 앞
    expect(t.remaining(37.5, 127.0, 8, at(240)), 4); // 20m 뒤로 걸어가 기다린다
    expect(t.remaining(null, null, 0, at(300)), 4); // 기다린 240초가 시간표에 반영된다
  });

  // 남은 정거장 수에는 시간표로 줄어든 값도 섞인다. 그 값으로 "지나온 정차" 를 정하면, 위치가 잠깐 끊긴 사이 시간표가
  // 한 정거장 줄여 놓은 뒤 돌아온 정상 표본이 "이미 지난 정차" 로 걸러져 기다린 시간을 영영 못 읽는다.
  test('시간표로 줄어든 값이 지연 관측을 막지 않는다', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(37.5, 127.0, 8, at(0)), 4);
    expect(t.remaining(null, null, 0, at(121)), 3); // 위치가 끊긴 사이 시간표가 줄인다
    expect(t.remaining(37.5, 127.0, 8, at(240)), 3); // 실은 아직 출발역이다
    expect(t.remaining(null, null, 0, at(361)), 3); // 기다린 240초가 반영돼 더 줄지 않는다
  });

  // offsetSec 은 도착 시각이고 출발 시각은 받아오지 않는다. 계획대로 서 있는 시간을 지연으로 세면 안 된다.
  test('정차 앞뒤 30m 안에서는 지연으로 재지 않는다', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(37.501, 127.0, 8, at(150)), 3); // 역1(계획 도착 120초) 위에 서 있다
    expect(t.remaining(null, null, 0, at(241)), 2); // 역2 계획 시각까지 그대로 센다

    // 떠난 쪽도 같다. 한쪽만 막으면 정차 시간이 그대로 지연으로 들어온다.
    final past = StopTracker(leg, line);
    expect(past.remaining(37.50118, 127.0, 8, at(150)), 3); // 역1 을 20m 지난 지점
    expect(past.remaining(null, null, 0, at(241)), 2);
  });

  test('오차가 큰 표본 하나가 값을 굳히지 않는다', () {
    final t = StopTracker(leg, line);
    expect(t.remaining(37.5, 127.0, 8, at(240)), 4);
    expect(t.remaining(37.5, 127.0, 90, at(246)), 4); // 시간표로 떨어져도 민 값이라 그대로
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
