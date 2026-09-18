import 'package:latlong2/latlong.dart';

import '../models/itinerary.dart';
import '../util/polyline.dart';
import 'geo.dart' as geo;

/// 안내 중 "지금 어느 구간인가"를 정한다. 위치가 현재 구간 끝점 반경 안에 들어오거나 다음 구간 경로선을 따라가기
/// 시작하면 다음 구간으로 넘기고, 사용자가 손으로 앞뒤로 옮길 수도 있다. 화면·업로더는 이 객체의 index 와 mode 만 본다.
class LegTracker {
  LegTracker(this.legs)
      : points = [
          for (final l in legs)
            l.polyline.isEmpty
                ? [LatLng(l.fromLat, l.fromLon), LatLng(l.toLat, l.toLon)]
                : decodePolyline(l.polyline)
        ];

  final List<Leg> legs;

  /// 구간별 경로선. 화면도 이 값을 쓴다(폴리라인 디코딩은 생성 때 한 번만).
  final List<List<LatLng>> points;
  int index = 0;

  // 구간 끝점 도달 반경 40m: 도심 GPS 오차(10~30m)보다 크고, 정류장 간격(최소 200m 안팎)보다 작다. 실기기 궤적으로 재보정.
  static const arriveRadiusM = 40.0;
  // 경로선 인계 판정(2026-09-18 초기값, 실기기 궤적으로 재보정):
  //   onRouteM 25 = 다음 구간 경로선 위로 본다. 도심 GPS 오차 상한 근처.
  //   handoffAlongM 40 = 그 경로선을 이만큼 따라갔을 때만. 두 구간이 맞닿은 지점(현재 끝=다음 시작)은 제외된다.
  //   offRouteM 60 = 현재 구간 경로선에서 이만큼 벗어났을 때만(지상 구간끼리 나란한 길 오판 방지).
  //   maxAccuracyM 50 = 실내에서 튀는 표본 차단.
  //   handoffHits 2 = 연속 두 번 맞아야 넘긴다.
  static const onRouteM = 25.0;
  static const handoffAlongM = 40.0;
  static const offRouteM = 60.0;
  static const maxAccuracyM = 50.0;
  static const handoffHits = 2;

  int _hits = 0;

  Leg get current => legs[index];
  List<LatLng> get currentPoints => points[index];
  bool get isLast => index >= legs.length - 1;

  /// 샘플에 붙일 수단. 도보 walk, 자전거(따릉이 포함) bicycle, 그 밖의 탑승 구간은 transit(속도 학습에서 제외).
  String get mode => modeOf(current);

  static String modeOf(Leg leg) {
    switch (leg.mode) {
      case 'WALK':
        return 'walk';
      case 'BICYCLE':
        return 'bicycle';
      default:
        return 'transit';
    }
  }

  /// 위치를 넣고 구간이 바뀌었으면 true. 마지막 구간 끝에 닿아도 index 는 마지막에 머문다(종료는 사용자가 누른다).
  /// 대중교통 구간의 끝점은 지하 승강장이라 GPS 가 반경 안에 들어오지 못하므로, 다음 구간 경로선을 따라가는
  /// 것으로도 넘긴다.
  bool update(double lat, double lon, {double accuracyM = 0}) {
    if (isLast) return false;
    if (distanceM(lat, lon, current.toLat, current.toLon) <= arriveRadiusM) {
      _advance();
      return true;
    }
    if (!_onNextLeg(lat, lon, accuracyM)) {
      _hits = 0;
      return false;
    }
    _hits++;
    if (_hits < handoffHits) return false;
    _advance();
    return true;
  }

  bool _onNextLeg(double lat, double lon, double accuracyM) {
    if (accuracyM > maxAccuracyM) return false;
    final next = geo.projectOnPolyline(lat, lon, points[index + 1]);
    if (next.distM > onRouteM || next.alongM < handoffAlongM) return false;
    // 대중교통 구간은 선로·도로가 다음 도보와 겹쳐 "현재 경로선 위"가 계속 참이므로 이 조건을 빼고 본다.
    if (current.transitLeg) return true;
    return geo.projectOnPolyline(lat, lon, currentPoints).distM >= offRouteM;
  }

  void _advance() {
    index++;
    _hits = 0;
  }

  void next() {
    if (!isLast) _advance();
  }

  void prev() {
    if (index > 0) {
      index--;
      _hits = 0;
    }
  }

  static double distanceM(double lat1, double lon1, double lat2, double lon2) =>
      geo.distanceM(lat1, lon1, lat2, lon2);
}
