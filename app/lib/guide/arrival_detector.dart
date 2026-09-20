/// 정확한 위치가 목적지 반경 안에 연속해서 들어오면 도착으로 판정한다.
class ArrivalDetector {
  ArrivalDetector({this.nearM = 40, this.hits = 3, this.maxAccuracyM = 50});

  final double nearM;
  final int hits;
  final double maxAccuracyM;

  int _near = 0;

  /// 목적지까지 거리가 distM 인 표본을 넣는다. 도착이 막 확정되면 true.
  /// 한 번 true 를 준 뒤에는 다시 주지 않는다 — 종료가 실패해도 매 표본마다 다시 부르지 않게.
  bool _fired = false;

  bool update(double distM, double accuracyM) {
    if (_fired || accuracyM > maxAccuracyM) return false;
    if (distM > nearM) {
      _near = 0;
      return false;
    }
    _near++;
    if (_near < hits) return false;
    _fired = true;
    return true;
  }

  /// 셈과 확정을 지운다. 사용자가 구간을 손으로 옮겨 마지막 구간을 벗어났을 때 쓴다.
  void reset() {
    _near = 0;
    _fired = false;
  }
}
