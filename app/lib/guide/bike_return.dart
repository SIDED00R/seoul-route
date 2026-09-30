import 'geo.dart' as geo;

/// 따릉이 구간에서 "계획과 다른 대여소에 반납하고 걷기 시작했다"를 위치만으로 판정한다. 폰 활동 인식은 자전거를
/// 준다는 보장이 없어(안내 궤적에 ON_BICYCLE 이 찍힌 적이 없다) 쓰지 않는다.
/// 믿을 만한 표본이 아래 넷을 minDuration 동안 이어서 만족하면 한 번 true 를 준다.
/// - 계획 반납 대여소에서 awayFromStationM 밖(가까이 왔으면 계획대로 반납하는 중이다)
/// - 계획 자전거 경로선에서 offRouteM 밖(경로선 위를 느리게 가는 건 자전거를 끌고 가는 것일 수 있다 — 한강 다리 보도)
/// - GPS 속도가 walkMps 를 넘는 표본이 overSamples 개 연속되지 않는다(0 은 속도를 모르는 것으로 보고 넘긴다). 걷는
///   표본도 제공자가 계산한 속도가 튀므로(에뮬레이터 fused 실측 0.85~1.80, 가끔 3.8) 표본 하나로 창을 지우지 않는다.
///   신호에 서며 타는 자전거는 타는 동안 이어지는 초과 표본이 창을 지운다
/// - 창 시작점에서 minMovedM 넘게 움직였고, 걷는 속도(walkMps × 경과)를 넘지 않는다. 창을 지우는 기준에는 위치 튐
///   여유 jitterM 을 더한다 — 2m/s 안팎으로 계속 타는 자전거(보고 속도가 0 이어도)는 이 평균으로 걸러진다
/// 임계값은 초기값(2026-09-30, 따릉이 주행 궤적 없이 걷기 1.2~1.5m/s·자전거 3~5m/s 를 가정)이다. overSamples 는 표본
/// 간격 2초 기준으로, 보고 속도 = 실제 × 0.71~1.5·표본 2% 는 3.8 튐 가정의 시뮬레이션에서 1.6m/s 걷기의 5분 안 판정률이
/// 3표본 24%·4표본 79%·5표본 97% 라 5 로 잡았다. 안내 궤적에 자전거 구간이 모이면 재보정한다(이슈 #108).
class BikeReturnDetector {
  static const awayFromStationM = 150.0;
  static const offRouteM = 60.0;
  static const walkMps = 1.8;
  static const overSamples = 5;
  static const maxAccuracyM = 50.0;
  static const minMovedM = 60.0;
  static const jitterM = 30.0;
  static const minDuration = Duration(seconds: 90);

  DateTime? _since;
  double _lat0 = 0, _lon0 = 0;
  int _over = 0;

  /// stationM 은 계획 반납 대여소까지, routeM 은 계획 경로선까지 거리. 조건이 깨지면 창을 지우고, 정확도가 나쁜 표본은
  /// 창을 건드리지 않는다.
  bool update({
    required double lat,
    required double lon,
    required double stationM,
    required double routeM,
    required double speedMps,
    required double accuracyM,
    required DateTime at,
  }) {
    if (accuracyM > maxAccuracyM) return false;
    _over = speedMps > walkMps ? _over + 1 : 0;
    if (stationM < awayFromStationM || routeM < offRouteM || _over >= overSamples) {
      reset();
      return false;
    }
    final since = _since;
    if (since == null) {
      _since = at;
      _lat0 = lat;
      _lon0 = lon;
      return false;
    }
    final elapsed = at.difference(since);
    final moved = geo.distanceM(_lat0, _lon0, lat, lon);
    if (moved > walkMps * elapsed.inSeconds + jitterM) {
      reset();
      return false;
    }
    if (elapsed < minDuration || moved < minMovedM || moved > walkMps * elapsed.inSeconds) return false;
    reset();
    return true;
  }

  void reset() {
    _since = null;
    _over = 0;
  }
}
