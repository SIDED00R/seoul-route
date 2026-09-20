/// 도보 구간에서 계획 경로를 벗어났는지 본다. 대중교통은 정해진 노선을 따라가고, 자전거는 양끝이 대여소로 묶여
/// 있어 둘 다 이 판정을 걸지 않는다(GuideSession._checkOffRoute).
///
/// 2026-09-20 초기값(계획 경로를 저장하지 않아 과거 궤적으로는 못 잰다 — 다음 외출 궤적으로 재보정):
///   awayM 60 = 경로선에서 이만큼 떨어진 표본을 벗어난 것으로 센다. LegTracker.offRouteM 과 같은 값이다.
///   hits 5 = 연속 이만큼이어야 이탈로 본다. 위치 요청 간격이 2초라 약 10초.
///   maxAccuracyM 50 = 이보다 오차가 큰 표본은 세지도, 셈을 지우지도 않는다(실내·지하에서 튀는 값).
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
