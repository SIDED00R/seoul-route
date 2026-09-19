/// 폰 활동 인식(안드로이드 Activity Recognition) 관측을 끈적하게 판정한다.
/// 플러그인(flutter_activity_recognition 4.0.0)은 판정(type·confidence 등급)이 **바뀔 때만** 이벤트를 준다 — 같은 판정이
/// 이어지는 동안은 아무 이벤트도 오지 않는다. 그래서 관측은 후보와 시작 시각만 기록하고, settle() 을 주기적으로 불러
/// 후보가 hold 이상 유지됐을 때 현재 활동으로 확정한다. 한 번 튄 판정은 곧 다른 판정 이벤트가 오며 지워진다.
/// LOW 신뢰도·UNKNOWN 은 후보가 되지 않고 후보도 지우지 않는다. STILL 도 후보가 되지는 않지만, stillHold 이상
/// 이어지면 확정 활동을 미상으로 되돌린다 — 그러지 않으면 열차처럼 폰이 계속 정지로 보는 동안 직전 활동이 그대로 남는다.
/// 값은 서버 traces.activity 와 같은 walk / bicycle / vehicle / unknown.
class ActivityClassifier {
  ActivityClassifier({
    this.hold = const Duration(seconds: 20),
    this.stillHold = const Duration(minutes: 2),
  });

  final Duration hold;

  /// 정지 판정이 이만큼 이어지면 확정 활동을 미상으로 되돌린다.
  /// 2026-09-19 실기기 궤적 1건 기준: 도보 구간의 정지 연속은 최대 18초, 대중교통 구간은 최대 2,161초였다.
  /// 신호 대기(2분 안팎)에서 미상으로 떨어지지 않도록 2분으로 둔다. 궤적이 더 모이면 재보정한다.
  final Duration stillHold;

  String current = 'unknown';
  String? _candidate;
  DateTime? _since;
  DateTime? _stillSince;

  /// 활동 인식 type(WALKING/RUNNING/ON_BICYCLE/IN_VEHICLE/STILL/UNKNOWN)·confidence(HIGH/MEDIUM/LOW) 를 넣는다.
  void observe(String type, String confidence, DateTime now) {
    if (confidence == 'LOW') return;
    final mapped = mapType(type);
    if (mapped == 'still') {
      _stillSince ??= now; // 정지가 이어진 시작 시각. 이미 재고 있으면 그대로 둔다
      return;
    }
    if (mapped == 'unknown') return;
    _stillSince = null; // 움직임 판정이 왔으면 정지가 끊겼다
    if (mapped == current) {
      _candidate = null;
      _since = null;
      return;
    }
    if (mapped != _candidate) {
      _candidate = mapped;
      _since = now;
    }
  }

  /// 위치 샘플마다, 그리고 위치가 끊긴 지하를 위해 10초 주기 점검에서도 부른다. 후보가 hold 이상 유지됐으면
  /// 확정하고, 그런 후보가 없는데 정지가 stillHold 이상 이어졌으면 미상으로 되돌린다. 확정 활동이 바뀌었으면 true.
  bool settle(DateTime now) {
    final c = _candidate, s = _since;
    if (c != null && s != null && now.difference(s) >= hold) {
      current = c;
      _candidate = null;
      _since = null;
      // 정지 시계는 건드리지 않는다. 움직임 관측이 이미 지우므로, 여기서 지우면 "움직임 판정이 잠깐 왔다가
      // 정지가 계속되는" 순서에서 시계가 다시 시작될 길이 없어(같은 판정은 이벤트가 오지 않는다) 감쇠가 죽는다.
      return true;
    }
    final still = _stillSince;
    if (still == null || current == 'unknown' || now.difference(still) < stillHold) return false;
    current = 'unknown';
    return true;
  }

  static String mapType(String type) {
    switch (type) {
      case 'WALKING':
      case 'RUNNING':
        return 'walk';
      case 'ON_BICYCLE':
        return 'bicycle';
      case 'IN_VEHICLE':
        return 'vehicle';
      case 'STILL':
        return 'still';
      default:
        return 'unknown';
    }
  }

  /// 안내 구간 수단(walk/bicycle/transit)과 판정이 어긋나는가. 미상은 어긋남이 아니고, 대중교통 구간의 걷기(역 구내
  /// 이동)도 정상이다.
  static bool mismatch(String legMode, String activity) {
    switch (activity) {
      case 'walk':
        return legMode == 'bicycle';
      case 'bicycle':
        return legMode != 'bicycle';
      case 'vehicle':
        return legMode != 'transit';
      default:
        return false;
    }
  }

  static String label(String activity) {
    switch (activity) {
      case 'walk':
        return '걷기';
      case 'bicycle':
        return '자전거';
      case 'vehicle':
        return '차량';
      default:
        return '미상';
    }
  }
}
