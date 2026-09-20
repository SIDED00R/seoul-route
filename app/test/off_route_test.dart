import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/off_route.dart';

void main() {
  test('연속으로 벗어나야 이탈로 본다', () {
    final d = OffRouteDetector();
    for (var i = 0; i < 4; i++) {
      expect(d.update(80, 10), isFalse, reason: '${i + 1}번째');
    }
    expect(d.update(80, 10), isTrue); // 5번째
    expect(d.update(80, 10), isFalse); // 확정 뒤에는 다시 주지 않는다
  });

  test('되돌아오면 셈을 지운다', () {
    final d = OffRouteDetector();
    for (var i = 0; i < 4; i++) {
      d.update(80, 10);
    }
    expect(d.update(10, 10), isFalse); // 경로 위로 복귀
    for (var i = 0; i < 4; i++) {
      expect(d.update(80, 10), isFalse);
    }
    expect(d.update(80, 10), isTrue);
  });

  test('오차가 큰 표본은 세지도 지우지도 않는다', () {
    final d = OffRouteDetector();
    for (var i = 0; i < 4; i++) {
      d.update(80, 10);
    }
    expect(d.update(80, 90), isFalse); // 세지 않는다
    expect(d.update(10, 90), isFalse); // 경로 위여도 지우지 않는다
    expect(d.update(80, 10), isTrue); // 앞선 4번이 살아 있어 이번이 5번째
  });

  test('복귀한 뒤 다시 벗어나면 또 알린다', () {
    final d = OffRouteDetector(hits: 2);
    d.update(80, 10);
    expect(d.update(80, 10), isTrue);
    d.update(5, 10);
    d.update(80, 10);
    expect(d.update(80, 10), isTrue);
  });

  test('reset 은 확정 상태까지 지운다', () {
    final d = OffRouteDetector(hits: 2);
    d.update(80, 10);
    expect(d.update(80, 10), isTrue);
    d.reset();
    d.update(80, 10);
    expect(d.update(80, 10), isTrue);
  });
}
