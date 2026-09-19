import 'package:flutter_test/flutter_test.dart';
import 'package:seoul_route/guide/activity_classifier.dart';
import 'package:seoul_route/models/trace_sample.dart';

/// 플러그인처럼 판정이 바뀔 때만 observe 를 부르고, 위치 샘플처럼 5초마다 settle 을 부르는 흉내.
class _Stream {
  _Stream(this.c);

  final ActivityClassifier c;
  DateTime now = DateTime.utc(2026, 9, 15, 5);
  String? _lastType, _lastConf;

  /// 활동 인식이 (type, confidence) 를 판정한 채 [seconds] 동안 유지된다. 이벤트는 판정이 바뀐 순간 한 번만 간다.
  /// 그동안 5초마다 settle 을 부르고, 그 사이 current 가 바뀐 적이 있으면 true.
  bool hold(String type, String conf, int seconds) {
    if (type != _lastType || conf != _lastConf) {
      c.observe(type, conf, now);
      _lastType = type;
      _lastConf = conf;
    }
    var changed = false;
    for (var t = 0; t < seconds; t += 5) {
      now = now.add(const Duration(seconds: 5));
      changed |= c.settle(now);
    }
    return changed;
  }
}

void main() {
  test('같은 판정이 이벤트 없이 20초 유지되면 확정된다(첫 판정 포함)', () {
    final s = _Stream(ActivityClassifier());
    expect(s.hold('WALKING', 'HIGH', 15), isFalse);
    expect(s.c.current, 'unknown');
    expect(s.hold('WALKING', 'HIGH', 10), isTrue);
    expect(s.c.current, 'walk');
    // 차량 탑승 후 IN_VEHICLE/HIGH 가 10분간 유지돼도 이벤트는 처음 한 번뿐 → 그래도 전환돼야 한다
    expect(s.hold('IN_VEHICLE', 'HIGH', 600), isTrue);
    expect(s.c.current, 'vehicle');
  });

  test('잠깐 튄 판정은 다음 판정 이벤트가 지우고, 정지·LOW·UNKNOWN 은 후보를 건드리지 않는다', () {
    final s = _Stream(ActivityClassifier());
    s.hold('WALKING', 'HIGH', 30);
    expect(s.c.current, 'walk');
    expect(s.hold('IN_VEHICLE', 'HIGH', 10), isFalse); // 10초 뒤 다시 걷기 판정
    expect(s.hold('WALKING', 'MEDIUM', 30), isFalse);
    expect(s.c.current, 'walk');
    // 신호 대기로 차량↔정지가 번갈아 와도 차량 후보의 시작 시각은 유지된다
    expect(s.hold('IN_VEHICLE', 'HIGH', 10), isFalse);
    expect(s.hold('STILL', 'HIGH', 5), isFalse);
    expect(s.hold('IN_VEHICLE', 'LOW', 0), isFalse);
    expect(s.hold('UNKNOWN', 'HIGH', 0), isFalse);
    expect(s.hold('IN_VEHICLE', 'HIGH', 10), isTrue);
    expect(s.c.current, 'vehicle');
  });

  // 열차가 매끄럽게 달리면 폰은 대부분 STILL 을 준다. 그동안 승강장까지 걸어온 '걷기' 가 남아 있으면 안 된다.
  test('정지가 2분 넘게 이어지면 미상으로 되돌린다', () {
    final s = _Stream(ActivityClassifier());
    s.hold('WALKING', 'HIGH', 30);
    expect(s.c.current, 'walk');
    expect(s.hold('STILL', 'HIGH', 115), isFalse); // 신호 대기 정도로는 그대로
    expect(s.c.current, 'walk');
    expect(s.hold('STILL', 'HIGH', 10), isTrue);
    expect(s.c.current, 'unknown');
    // 다시 움직이면 평소대로 확정된다.
    expect(s.hold('IN_VEHICLE', 'HIGH', 30), isTrue);
    expect(s.c.current, 'vehicle');
  });

  // 열차에서 실제로 오는 순서: 걷기 확정 → IN_VEHICLE 이 잠깐 → 다시 정지가 계속. 차량 후보가 확정될 때 정지 시계를
  // 지우면, 같은 판정은 이벤트가 오지 않으므로 시계가 다시 시작될 길이 없어 감쇠가 영영 안 돈다.
  test('움직임 판정이 잠깐 왔다가 정지가 이어져도 감쇠가 돈다', () {
    final s = _Stream(ActivityClassifier());
    s.hold('WALKING', 'HIGH', 30);
    expect(s.c.current, 'walk');
    s.hold('IN_VEHICLE', 'HIGH', 10); // 차량 후보만 생기고 아직 확정 전
    s.hold('STILL', 'HIGH', 10); // 정지로 바뀌어도 차량 후보는 살아남아 20초에 확정된다
    expect(s.c.current, 'vehicle');
    expect(s.hold('STILL', 'HIGH', 120), isTrue); // 정지 시계는 이어진다
    expect(s.c.current, 'unknown');
  });

  test('정지가 끊기면 다시 처음부터 잰다', () {
    final s = _Stream(ActivityClassifier());
    s.hold('WALKING', 'HIGH', 30);
    expect(s.hold('STILL', 'HIGH', 110), isFalse);
    expect(s.hold('WALKING', 'HIGH', 10), isFalse); // 같은 활동이라 확정은 그대로
    expect(s.hold('STILL', 'HIGH', 110), isFalse); // 정지 시계가 다시 시작됐다
    expect(s.c.current, 'walk');
  });

  test('불일치 판정: 대중교통 구간의 걷기는 정상, 도보 구간의 차량·자전거는 불일치', () {
    expect(ActivityClassifier.mismatch('transit', 'walk'), isFalse);
    expect(ActivityClassifier.mismatch('walk', 'walk'), isFalse);
    expect(ActivityClassifier.mismatch('walk', 'vehicle'), isTrue);
    expect(ActivityClassifier.mismatch('walk', 'bicycle'), isTrue);
    expect(ActivityClassifier.mismatch('bicycle', 'walk'), isTrue);
    expect(ActivityClassifier.mismatch('transit', 'vehicle'), isFalse);
    expect(ActivityClassifier.mismatch('walk', 'unknown'), isFalse);
  });

  test('샘플 JSON 은 activity 가 있을 때만 싣는다', () {
    final base = TraceSample(ts: DateTime.utc(2026, 9, 15), lat: 37.5, lon: 127.0, accuracyM: 5, mode: 'walk');
    expect(base.toJson().containsKey('activity'), isFalse);
    final withAct = TraceSample(
        ts: DateTime.utc(2026, 9, 15), lat: 37.5, lon: 127.0, accuracyM: 5, mode: 'walk', activity: 'vehicle');
    expect(withAct.toJson()['activity'], 'vehicle');
  });
}
