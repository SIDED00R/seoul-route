import 'geo.dart' as geo;

/// 지하철·철도로 가는 동안 계획과 다르게 움직이는 두 경우.
/// - walkedAway: 계획하지 않은 역에서 내려(환승 통로에서는 역 밖으로 나와) 걷는 중
/// - otherTrain: 계획과 다른 열차(다른 노선·반대 방향·내릴 역을 지나침)로 가는 중
enum TransitDeviation { none, walkedAway, otherTrain }

/// 지하철·철도로 가는 동안 계획과 다르게 움직이는지 위치로 판정한다. 폰 활동 인식은 판정 근거로 쓰지 않는다 —
/// 2026-11-04·10-09 운영 안내 궤적에서 열차 안 판정이 내내 UNKNOWN LOW 이거나 타기 전 WALKING 이 그대로 남아 있었다.
/// 탈것 안(riding)이면 walkedAway 를 보류만 한다(구간 넘김 보류와 같은 기준).
/// 같은 궤적에서 저장된 표본(5초로 솎음) 간격은 중앙값 6초였고, 지하 역 사이에서는 40~50초 끊겼다. 지하 위치는 역에서
/// 와이파이로 정확도 10~30m 로 잡혔고 정차 1분 안에 최대 121m 흔들렸다(정확도 8m 이하인 지하 표본은 없었다).
/// 정확도 20m 안팎으로 1~2km 뒤의 지난 역 자리가 다시 찍히기도 했다. 고가 구간(신대방)에서는 움직이는 열차도
/// 정확도 4~12m 로 잡혔다.
///
/// walkedAway — 지상 GNSS 로 보이는 표본(gnssAccuracyM 이하)으로 창을 연다(지하 와이파이 흔들림은 창에 들지 않는다).
/// 아래를 만족한 채 walkDuration·walkMovedM·walkFixes 를 넘기면 판정한다. [BikeReturnDetector] 와 같은 창 방식이다.
/// - 정확도 stepAccuracyM 이하 표본끼리 위치 오차를 빼고도 stepMps 넘게 움직이지 않았다(넘으면 창을 지운다 — 고가
///   구간에서 출발하는 열차). 걸음 검사 기준 표본과 stepGap 안의 표본은 walkedAway 에서 건너뛴다
/// - 탑승역·계획 하차역 둘 다에서 stopClearM 밖(그 안은 출입구↔승강장 도보라 기존 구간 넘김이 맡는다. 출입구는 승강장
///   350m 안의 것만 잇는다 — gtfs pathways)
/// - 창 시작점에서 걷는 속도(walkMps × 경과 + 위치 튐 여유 jitterM)를 넘게 오지 않았다. 판정 때는 여유 없이 걷는
///   속도 이하여야 한다
/// - 탈것 안(riding)이 아니다
///
/// otherTrain — 믿을 만한 표본(trainAccuracyM 이하) 중 계획 경로(지하철·철도·역 안 구간들과 지금 구간)에서 offLineM 넘게
/// 떨어졌고 기준점에서 그 자리까지 평균 trainMps 넘게 온 표본이 trainHits 개 연속이면 판정한다. 기준점은 계획 경로
/// offLineM 안에 마지막으로 잡힌 표본이고, 아직 없으면 구간에 들어온 때의 탑승역이다(탑승역 없이 시작했으면 기준점이
/// 생길 때까지 세지 않는다). 걸어서는 그 속도로 경로에서 멀어질 수 없다. 지난 역 자리가 다시 찍히는 튐과 구간이 늦게 따라와 뒤 계획 구간 위에 찍히는 표본은 계획 경로 위라
/// 셈을 지운다.
///
/// 임계값은 초기값(2026-11-04, 위 궤적 2건과 그 정차·고가 구간을 늘린 합성 궤적)이다. 궤적이 모이면 재보정한다.
class TransitDeviationDetector {
  static const gnssAccuracyM = 8.0;
  static const stepAccuracyM = 30.0;
  static const stepMps = 3.0;

  /// 걸음 검사 표본 간격 하한. 위치 오차 여유가 간격과 무관하게 고정이라 위치 요청 간격(2초)으로 재면 문턱이 정확도
  /// 8m 기준 11m/s 까지 올라 출발하는 열차를 놓친다. 임계값을 잡은 저장 궤적과 같은 간격(TraceUploader 5초 솎음, 간격
  /// 중앙값 6초 — 2026-11-04)으로 맞춘다. 궤적이 모이면 재보정한다.
  static const stepGap = Duration(seconds: 5);
  static const stopClearM = 400.0;
  static const walkMps = 1.8;
  static const jitterM = 30.0;
  static const walkMovedM = 100.0;
  static const walkDuration = Duration(seconds: 60);
  static const walkFixes = 5;

  static const trainAccuracyM = 50.0;
  static const offLineM = 400.0;
  static const trainMps = 3.0;
  static const trainHits = 2;

  DateTime? _since;
  double _lat0 = 0, _lon0 = 0;
  int _fixes = 0;

  DateTime? _stepAt;
  double _stepLat = 0, _stepLon = 0, _stepAcc = 0;

  double _refLat = 0, _refLon = 0;
  DateTime? _refAt;
  int _offHits = 0;

  /// 구간에 들어왔을 때 부른다. 탑승역(boardLat·boardLon)을 들어온 시각(at)의 다른 열차 판정 기준점으로 둔다. 탑승역을
  /// 주지 않으면 계획 경로 위 표본이 잡힐 때까지 기준점이 없어 다른 열차를 세지 않는다.
  void start({required DateTime at, double? boardLat, double? boardLon}) {
    _since = null;
    _stepAt = null;
    _offHits = 0;
    if (boardLat == null || boardLon == null) {
      _refAt = null;
      return;
    }
    _refLat = boardLat;
    _refLon = boardLon;
    _refAt = at;
  }

  /// 표본 하나를 넣는다. pathM 은 계획 경로까지, boardM·alightM 은 지금 구간의 탑승역·계획 하차역까지 거리.
  /// watchWalk 가 거짓이면(내려서 걷는 구간) walkedAway 는 보지 않는다. 판정이 막 나면 그 종류를 주고 셈을 지운다.
  TransitDeviation update({
    required double lat,
    required double lon,
    required double accuracyM,
    required DateTime at,
    required double pathM,
    required double boardM,
    required double alightM,
    required bool riding,
    required bool watchWalk,
  }) {
    if (watchWalk && _walkedAway(lat, lon, accuracyM, at, boardM, alightM, riding)) {
      _clear(lat, lon, at);
      return TransitDeviation.walkedAway;
    }
    if (_otherTrain(lat, lon, accuracyM, at, pathM)) {
      _clear(lat, lon, at);
      return TransitDeviation.otherTrain;
    }
    return TransitDeviation.none;
  }

  bool _walkedAway(double lat, double lon, double accuracyM, DateTime at, double boardM, double alightM, bool riding) {
    if (accuracyM > stepAccuracyM) return false;
    final stepAt = _stepAt;
    if (stepAt != null && at.isAfter(stepAt) && at.difference(stepAt) < stepGap) return false;
    final jumped = stepAt != null &&
        at.isAfter(stepAt) &&
        geo.distanceM(_stepLat, _stepLon, lat, lon) - (_stepAcc + accuracyM) >
            stepMps * at.difference(stepAt).inMilliseconds / 1000;
    _stepAt = at;
    _stepLat = lat;
    _stepLon = lon;
    _stepAcc = accuracyM;
    if (jumped) {
      _since = null;
      return false;
    }
    if (accuracyM > gnssAccuracyM) return false;
    if (riding || boardM < stopClearM || alightM < stopClearM) {
      _since = null;
      return false;
    }
    final since = _since;
    final seconds = since == null ? 0 : at.difference(since).inMilliseconds / 1000;
    final moved = since == null ? 0 : geo.distanceM(_lat0, _lon0, lat, lon);
    if (since == null || moved > walkMps * seconds + jitterM) {
      _since = at;
      _lat0 = lat;
      _lon0 = lon;
      _fixes = 1;
      return false;
    }
    _fixes++;
    return seconds >= walkDuration.inSeconds &&
        moved >= walkMovedM &&
        moved <= walkMps * seconds &&
        _fixes >= walkFixes;
  }

  bool _otherTrain(double lat, double lon, double accuracyM, DateTime at, double pathM) {
    if (accuracyM > trainAccuracyM) return false;
    if (pathM <= offLineM) {
      _refLat = lat;
      _refLon = lon;
      _refAt = at;
      _offHits = 0;
      return false;
    }
    final refAt = _refAt;
    final seconds = refAt == null ? 0 : at.difference(refAt).inMilliseconds / 1000;
    if (seconds <= 0 || geo.distanceM(_refLat, _refLon, lat, lon) / seconds < trainMps) {
      _offHits = 0;
      return false;
    }
    return ++_offHits >= trainHits;
  }

  void _clear(double lat, double lon, DateTime at) {
    _since = null;
    _stepAt = null;
    _refLat = lat;
    _refLon = lon;
    _refAt = at;
    _offHits = 0;
  }
}
