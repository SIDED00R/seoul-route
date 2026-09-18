import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/step_tracker.dart';
import 'package:seoul_route/models/leg_detail.dart';

// 경도 0.0001도 ≈ 8.8m(위도 37.5), 위도 0.0001도 ≈ 11.1m.
WalkStep step(String dir, double lat, double lon, double distanceM, {String street = ''}) =>
    WalkStep(dir: dir, lat: lat, lon: lon, distanceM: distanceM, street: street);

void main() {
  // 북쪽으로 222m 가서 우회전, 동쪽으로 88m 가서 다시 우회전, 남쪽으로 111m 가서 도착.
  final steps = [
    step('DEPART', 37.5, 127.0, 222, street: '테헤란로'),
    step('RIGHT', 37.502, 127.0, 88),
    step('RIGHT', 37.502, 127.001, 111),
  ];
  StepTracker tracker() => StepTracker(steps, endLat: 37.501, endLon: 127.001);

  test('모퉁이 반경 안에 들어오면 다음 단계, 밖이면 그대로', () {
    final t = tracker();
    expect(t.update(37.5005, 127.0), isFalse);
    expect(t.index, 0);
    expect(t.update(37.50186, 127.0), isFalse); // 모퉁이에서 16m
    expect(t.index, 0);
    expect(t.update(37.50188, 127.0), isTrue); // 13m
    expect(t.index, 1);
  });

  test('한 번에 두 모퉁이를 지나면 두 단계 넘어간다', () {
    final t = tracker();
    expect(t.update(37.502, 127.001), isTrue);
    expect(t.index, 2);
  });

  test('모퉁이를 놓쳐도 뒤 단계 선분 위로 들어오면 따라잡는다', () {
    final t = tracker();
    // 마지막 단계(남쪽으로 내려가는 선분) 위이고 첫 단계 선분에서 88m 떨어진 지점
    expect(t.update(37.5015, 127.001), isTrue);
    expect(t.index, 2);
    expect(t.update(37.5015, 127.001), isFalse); // 같은 자리면 그대로
    expect(t.update(37.5, 127.0), isFalse); // 되돌아가도 단계는 뒤로 가지 않는다
    expect(t.index, 2);
  });

  test('남은 거리는 단계 길이를 넘지 않는다', () {
    final t = tracker();
    expect(t.remainM(37.5, 127.0), closeTo(222, 2)); // 출발점에서 모퉁이까지
    expect(t.remainM(37.49, 127.0), 222); // 훨씬 뒤에 있어도 단계 길이로 자른다
    expect(t.remainM(37.5018, 127.0), closeTo(22, 2));
  });

  test('오차가 큰 표본으로는 단계를 넘기지 않는다', () {
    final t = tracker();
    // 정확도가 좋았다면 마지막 단계로 건너뛸 자리(3번 단계 선분 위, 1번 단계 선분에서 88m)
    expect(t.update(37.5015, 127.001, accuracyM: 100), isFalse);
    expect(t.index, 0);
    expect(t.update(37.5015, 127.001, accuracyM: 50), isTrue); // 상한(50m)까지는 쓴다
    expect(t.index, 2);
  });

  test('단계가 없거나 하나뿐이면 움직이지 않는다', () {
    final empty = StepTracker(const [], endLat: 37.5, endLon: 127.0);
    expect(empty.isEmpty, isTrue);
    expect(empty.update(37.5, 127.0), isFalse);
    expect(empty.remainM(37.5, 127.0), 0);
    final one = StepTracker([steps.first], endLat: 37.502, endLon: 127.0);
    expect(one.update(37.502, 127.0), isFalse);
    expect(one.index, 0);
  });
}
