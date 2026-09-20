/// 마지막 구간에서 목적지에 닿았는지 본다. 참이 되면 GuideSession 이 사용자가 누르지 않아도 안내를 끝낸다.
///
/// 2026-09-20 초기값(실기기 궤적으로 재보정):
///   nearM 40 = 목적지에서 이만큼 안이면 닿은 것으로 센다. LegTracker.arriveRadiusM 과 같은 값이다.
///   hits 3 = 연속 이만큼이어야 도착으로 본다. 위치 요청 간격이 2초라 약 6초.
///   maxAccuracyM 50 = 이보다 오차가 큰 표본은 세지도, 셈을 지우지도 않는다(건물 안에서 튀는 값).
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
