import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/arrival_detector.dart';

void main() {
  test('목적지 반경 안 표본이 연속 hits 번이어야 도착이다', () {
    final d = ArrivalDetector();
    expect(d.update(20, 8), isFalse);
    expect(d.update(20, 8), isFalse);
    expect(d.update(20, 8), isTrue);
  });

  test('중간에 멀어지면 셈을 다시 시작한다', () {
    final d = ArrivalDetector();
    d.update(20, 8);
    d.update(20, 8);
    expect(d.update(45, 8), isFalse); // 반경 밖
    expect(d.update(20, 8), isFalse);
    expect(d.update(20, 8), isFalse);
    expect(d.update(20, 8), isTrue);
  });

  test('오차가 큰 표본은 세지도, 셈을 지우지도 않는다', () {
    final d = ArrivalDetector();
    d.update(20, 8);
    expect(d.update(20, 80), isFalse); // 세지 않는다
    expect(d.update(500, 80), isFalse); // 멀어도 셈을 지우지 않는다
    expect(d.update(20, 8), isFalse);
    expect(d.update(20, 8), isTrue);
  });

  test('한 번 도착이라고 한 뒤에는 다시 참을 주지 않는다', () {
    final d = ArrivalDetector();
    for (var i = 0; i < 3; i++) {
      d.update(10, 8);
    }
    for (var i = 0; i < 5; i++) {
      expect(d.update(10, 8), isFalse);
    }
  });

  test('reset 뒤에는 다시 셀 수 있다', () {
    final d = ArrivalDetector();
    for (var i = 0; i < 3; i++) {
      d.update(10, 8);
    }
    d.reset();
    expect(d.update(10, 8), isFalse);
    expect(d.update(10, 8), isFalse);
    expect(d.update(10, 8), isTrue);
  });
}
