import 'geo.dart' as geo;

/// 따릉이 구간에서 "계획과 다른 대여소에 반납하고 걷기 시작했다"를 위치만으로 판정한다. 폰 활동 인식은 자전거를
/// 준다는 보장이 없어(안내 궤적에 ON_BICYCLE 이 찍힌 적이 없다) 쓰지 않는다.
/// 믿을 만한 표본이 아래 넷을 minDuration 동안 이어서 만족하면 한 번 true 를 준다.
/// - 계획 반납 대여소에서 awayFromStationM 밖(가까이 왔으면 계획대로 반납하는 중이다)
/// - 계획 자전거 경로선에서 offRouteM 밖(경로선 위를 느리게 가는 건 자전거를 끌고 가는 것일 수 있다 — 한강 다리 보도)
/// - GPS 속도 maxSpeedMps 이하(0 은 속도를 모르는 것으로 보고 넘긴다)
/// - 창 시작점에서 minMovedM 넘게 움직였고, 걷는 속도(maxSpeedMps × 경과 + 위치 튐 여유 jitterM)를 넘지 않는다
/// 임계값은 초기값(2026-09-30, 따릉이 주행 궤적 없이 걷기 1.2~1.5m/s·자전거 3~5m/s 를 가정)이다. 안내 궤적에 자전거
/// 구간이 모이면 재보정한다(이슈 #108).
class BikeReturnDetector {
  static const awayFromStationM = 150.0;
  static const offRouteM = 60.0;
  static const maxSpeedMps = 1.8;
  static const maxAccuracyM = 50.0;
  static const minMovedM = 60.0;
  static const jitterM = 30.0;
  static const minDuration = Duration(seconds: 90);

  DateTime? _since;
  double _lat0 = 0, _lon0 = 0;

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
    if (stationM < awayFromStationM || routeM < offRouteM || speedMps > maxSpeedMps) {
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
    if (moved > maxSpeedMps * elapsed.inSeconds + jitterM) {
      reset();
      return false;
    }
    if (elapsed < minDuration || moved < minMovedM) return false;
    reset();
    return true;
  }

  void reset() {
    _since = null;
  }
}
