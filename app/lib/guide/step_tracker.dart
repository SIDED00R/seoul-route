import 'package:latlong2/latlong.dart';

import '../models/leg_detail.dart';
import 'geo.dart';

/// 도보·자전거 구간 안에서 "지금 몇 번째 안내 단계인가"를 정한다. 단계 i 의 선분은 steps[i] 시작점에서
/// steps[i+1] 시작점까지(마지막 단계는 구간 끝점까지)다. 단계는 앞으로만 간다.
class StepTracker {
  StepTracker(this.steps, {required this.endLat, required this.endLon});

  final List<WalkStep> steps;
  final double endLat;
  final double endLon;
  int index = 0;

  // 2026-09-18 초기값(실기기 궤적으로 재보정):
  //   cornerM 15 = 다음 단계 시작점(모퉁이)에 닿았다고 보는 반경.
  //   onSegmentM 20 / offSegmentM 40 = 모퉁이를 못 잡고 지나쳤을 때, 뒤 단계 선분 위이고 현재 단계 선분에서
  //   벗어났으면 그 단계로 건너뛴다.
  //   maxAccuracyM 50 = 이보다 오차가 큰 표본으로는 단계를 넘기지 않는다(LegTracker·StopTracker 와 같은 값).
  //   단계는 앞으로만 가므로 한 번 잘못 넘기면 그 구간 내내 어긋난다. 2026-09-17 실기기 외출 궤적의 도보 표본 1,000개 중
  //   26개(2.6%)가 50m 초과였다.
  static const cornerM = 15.0;
  static const onSegmentM = 20.0;
  static const offSegmentM = 40.0;
  static const maxAccuracyM = 50.0;

  bool get isEmpty => steps.isEmpty;
  WalkStep? get current => steps.isEmpty ? null : steps[index];
  WalkStep? get next => index + 1 < steps.length ? steps[index + 1] : null;

  /// 위치를 넣고 단계가 바뀌었으면 true. 오차가 큰 표본은 무시한다.
  bool update(double lat, double lon, {double accuracyM = 0}) {
    if (steps.length < 2 || accuracyM > maxAccuracyM) return false;
    final before = index;
    var moved = true;
    while (moved) {
      moved = false;
      while (index + 1 < steps.length &&
          distanceM(lat, lon, steps[index + 1].lat, steps[index + 1].lon) <= cornerM) {
        index++;
        moved = true;
      }
      // 모퉁이를 못 잡고 지나쳤을 때: 현재 단계 선분에서 벗어났고 뒤 단계 선분 위면 그 단계로 건너뛴다.
      if (!moved && _segmentDist(lat, lon, index) >= offSegmentM) {
        for (var j = index + 1; j < steps.length; j++) {
          if (_segmentDist(lat, lon, j) <= onSegmentM) {
            index = j;
            moved = true;
            break;
          }
        }
      }
    }
    return index != before;
  }

  /// 현재 단계가 끝나는 지점(다음 모퉁이, 마지막이면 구간 끝)까지 남은 거리. 단계 길이를 넘지 않게 자른다.
  double remainM(double lat, double lon) {
    if (steps.isEmpty) return 0;
    final end = _segmentEnd(index);
    final d = distanceM(lat, lon, end.latitude, end.longitude);
    final len = steps[index].distanceM;
    return len > 0 && d > len ? len : d;
  }

  LatLng _segmentEnd(int i) =>
      i + 1 < steps.length ? LatLng(steps[i + 1].lat, steps[i + 1].lon) : LatLng(endLat, endLon);

  double _segmentDist(double lat, double lon, int i) {
    final end = _segmentEnd(i);
    return projectOnPolyline(lat, lon, [LatLng(steps[i].lat, steps[i].lon), end]).distM;
  }
}
