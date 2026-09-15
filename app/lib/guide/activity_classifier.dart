/// 폰 활동 인식(안드로이드 Activity Recognition) 관측을 끈적하게 판정한다.
/// 플러그인(flutter_activity_recognition 4.0.0)은 판정(type·confidence 등급)이 **바뀔 때만** 이벤트를 준다 — 같은 판정이
/// 이어지는 동안은 아무 이벤트도 오지 않는다. 그래서 관측은 후보와 시작 시각만 기록하고, 위치 샘플마다 settle() 을 불러
/// 후보가 hold 이상 유지됐을 때 현재 활동으로 확정한다. 한 번 튄 판정은 곧 다른 판정 이벤트가 오며 지워진다.
/// LOW 신뢰도·UNKNOWN·STILL 은 후보가 되지 않고 후보도 지우지 않는다(정지는 어느 쪽 불일치 판정에도 안 쓰인다).
/// 값은 서버 traces.activity 와 같은 walk / bicycle / vehicle / unknown.
class ActivityClassifier {
  ActivityClassifier({this.hold = const Duration(seconds: 20)});

  final Duration hold;
  String current = 'unknown';
  String? _candidate;
  DateTime? _since;

  /// 활동 인식 type(WALKING/RUNNING/ON_BICYCLE/IN_VEHICLE/STILL/UNKNOWN)·confidence(HIGH/MEDIUM/LOW) 를 넣는다.
  void observe(String type, String confidence, DateTime now) {
    if (confidence == 'LOW') return;
    final mapped = mapType(type);
    if (mapped == 'unknown' || mapped == 'still') return;
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

  /// 위치 샘플마다 부른다. 후보가 hold 이상 유지됐으면 확정하고 true.
  bool settle(DateTime now) {
    final c = _candidate, s = _since;
    if (c == null || s == null || now.difference(s) < hold) return false;
    current = c;
    _candidate = null;
    _since = null;
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
