import 'package:latlong2/latlong.dart';

import '../models/itinerary.dart';
import '../models/leg_detail.dart';
import 'geo.dart';

/// 대중교통 구간에서 "하차까지 몇 정거장 남았나"를 센다(하차역 포함이라 최소 1). 지상에서는 위치를 경로선에
/// 투영해 지나온 정차를 빼고, 지하라 위치가 멈춘 구간에서는 시간표(정차별 출발 기준 초)로 센다.
/// 한 번 줄어든 값은 다시 늘지 않는다 — 튀는 표본으로 "다음 역" 안내가 두 번 나가지 않게.
class StopTracker {
  /// shift 는 계획보다 밀린 시간(LegTracker.shift) — 다음 차를 탔으면 정차 시각도 그만큼 늦다.
  StopTracker(Leg leg, List<LatLng> points, {Duration shift = Duration.zero})
      : stops = leg.stops,
        _legStart = DateTime.tryParse(leg.start)?.add(shift),
        _alongStop = [
          for (final s in leg.stops) projectOnPolyline(s.lat, s.lon, points).alongM,
        ],
        _points = points;

  final List<TransitStop> stops;
  final DateTime? _legStart;
  final List<double> _alongStop;
  final List<LatLng> _points;

  // 경로선에서 이만큼 안쪽일 때만 위치로 진행을 센다(지하철은 지상 구간에서만 잡힌다).
  static const onRouteM = 80.0;
  static const maxAccuracyM = 50.0;
  // 정차를 지났다고 보는 여유(m). 정차 바로 옆에서 앞뒤로 흔들려도 한 번만 줄어든다.
  static const passedM = 30.0;

  int _remaining = -1;

  /// 남은 정거장 수(하차역 포함). 위치가 없으면 시간표만 쓴다.
  int remaining(double? lat, double? lon, double accuracyM, DateTime now) {
    final n = _byPosition(lat, lon, accuracyM) ?? _bySchedule(now) ?? stops.length + 1;
    if (_remaining < 0 || n < _remaining) _remaining = n;
    return _remaining;
  }

  /// 다음 정차 이름. 남은 정거장이 하나면(다음이 하차역) null.
  String? nextStopName() {
    if (_remaining <= 1 || stops.isEmpty) return null;
    final i = stops.length - (_remaining - 1);
    return i >= 0 && i < stops.length ? stops[i].name : null;
  }

  int? _byPosition(double? lat, double? lon, double accuracyM) {
    if (lat == null || lon == null || accuracyM > maxAccuracyM || _points.length < 2) return null;
    final here = projectOnPolyline(lat, lon, _points);
    if (here.distM > onRouteM) return null;
    var left = 1; // 하차역
    for (final along in _alongStop) {
      if (along > here.alongM + passedM) left++;
    }
    return left;
  }

  int? _bySchedule(DateTime now) {
    final start = _legStart;
    if (start == null) return null;
    var left = 1;
    for (final s in stops) {
      if (s.offsetSec > 0 && start.add(Duration(seconds: s.offsetSec)).isAfter(now)) left++;
    }
    return left;
  }
}
