/// 도보 위치가 경로선에서 60m 이상 벗어난 상태로 5회 이어지면 이탈로 판정한다.
/// 정확도 50m 초과 표본은 무시한다.
class OffRouteDetector {
  OffRouteDetector({this.awayM = 60, this.hits = 5, this.maxAccuracyM = 50});

  final double awayM;
  final int hits;
  final double maxAccuracyM;

  int _away = 0;
  bool _fired = false;

  /// 경로선까지 거리가 distM 인 표본을 넣는다. 이탈이 막 확정되면 true. 확정 뒤에는 다시 true 를 주지 않는다.
  bool update(double distM, double accuracyM) {
    if (accuracyM > maxAccuracyM) return false;
    if (distM < awayM) {
      _away = 0;
      _fired = false;
      return false;
    }
    _away++;
    if (_fired || _away < hits) return false;
    _fired = true;
    return true;
  }

  /// 구간이 바뀌거나 경로를 갈아 끼웠을 때 셈을 지운다.
  void reset() {
    _away = 0;
    _fired = false;
  }
}
