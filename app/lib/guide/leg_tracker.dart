import 'dart:math';

import '../models/itinerary.dart';

/// 안내 중 "지금 어느 구간인가"를 정한다. 위치가 현재 구간 끝점 반경 안에 들어오면 다음 구간으로 넘기고,
/// 사용자가 손으로 앞뒤로 옮길 수도 있다. 화면·업로더는 이 객체의 index 와 mode 만 본다.
class LegTracker {
  LegTracker(this.legs);

  final List<Leg> legs;
  int index = 0;

  // 구간 끝점 도달 반경 40m: 도심 GPS 오차(10~30m)보다 크고, 정류장 간격(최소 200m 안팎)보다 작다. 실기기 궤적으로 재보정.
  static const arriveRadiusM = 40.0;

  Leg get current => legs[index];
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
  bool update(double lat, double lon) {
    if (isLast) return false;
    if (distanceM(lat, lon, current.toLat, current.toLon) <= arriveRadiusM) {
      index++;
      return true;
    }
    return false;
  }

  void next() {
    if (!isLast) index++;
  }

  void prev() {
    if (index > 0) index--;
  }

  static double distanceM(double lat1, double lon1, double lat2, double lon2) {
    const r = 6371000.0;
    final p1 = lat1 * pi / 180, p2 = lat2 * pi / 180;
    final dp = (lat2 - lat1) * pi / 180, dl = (lon2 - lon1) * pi / 180;
    final a = sin(dp / 2) * sin(dp / 2) + cos(p1) * cos(p2) * sin(dl / 2) * sin(dl / 2);
    return 2 * r * asin(sqrt(a));
  }
}
