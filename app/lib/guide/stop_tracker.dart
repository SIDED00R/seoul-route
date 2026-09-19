import 'package:latlong2/latlong.dart';

import '../models/itinerary.dart';
import '../models/leg_detail.dart';
import 'geo.dart';

/// 대중교통 구간에서 "하차까지 몇 정거장 남았나"를 센다(하차역 포함이라 최소 1). 지상에서는 위치를 경로선에
/// 투영해 지나온 정차를 빼고, 지하라 위치가 멈춘 구간에서는 시간표(정차별 출발 기준 초)로 센다.
/// 시간표는 위치로 읽은 지연(_delay)만큼 밀어서 센다 — 계획한 차를 놓치고 늦은 차를 타도 따라온다.
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

  /// 열차가 계획보다 이만큼 늦다고 본 값. 시간표 판정은 정차 시각을 이만큼 민다.
  Duration _delay = Duration.zero;

  /// 위치로 확인한 통과 정차 수(단조 증가). 시간표로 줄어든 값은 섞지 않는다 — 시간표가 앞서 줄여 놓으면 뒤에 돌아온
  /// 정상 표본이 "이미 지난 정차" 로 걸러져 지연을 못 읽는다.
  int _passedSeen = 0;

  /// 남은 정거장 수(하차역 포함). 위치가 없으면 시간표로 센다.
  int remaining(double? lat, double? lon, double accuracyM, DateTime now) {
    final alongM = _alongHere(lat, lon, accuracyM);
    int? n;
    if (alongM == null) {
      n = _bySchedule(now);
    } else {
      n = _countAfter(alongM);
      final passed = stops.length - n + 1;
      if (passed > _passedSeen) _passedSeen = passed;
      _observeDelay(alongM, now);
    }
    final value = n ?? stops.length + 1;
    if (_remaining < 0 || value < _remaining) _remaining = value;
    return _remaining;
  }

  /// 지금 위치를 계획 시간표 위의 한 점으로 바꿔 계획 시각과 견준다. 승강장에 서 있으면 진행이 0 이라 기다린 시간이
  /// 그대로 지연으로 잡히고, 정차 사이를 계획대로 달리는 동안에는 0 이다. 늦은 쪽으로만 고친다 — 계획보다 이른
  /// 열차는 위치 판정이 알아서 세고, 시간표를 당기면 하차 안내가 되레 일찍 나간다.
  void _observeDelay(double alongM, DateTime now) {
    final start = _legStart;
    if (start == null) return;
    // 위치로 이미 지난 정차보다 앞으로는 되돌리지 않는다. 뒤로 크게 튄 표본을 "지금 여기" 로 읽으면 지연이 크게
    // 잡혀 굳고, 이번에는 하차 안내가 되레 늦게 나간다. 바닥은 정차 단위로만 움직이므로 앞으로 튄 표본은 정거장
    // 간격만큼 튀어야 기준을 옮길 수 있다.
    final floorM = _passedSeen > 0 && _passedSeen <= stops.length ? _alongStop[_passedSeen - 1] : 0.0;
    final plannedSec = _plannedSec(alongM > floorM ? alongM : floorM);
    if (plannedSec == null) return;
    final late = now.difference(start.add(Duration(seconds: plannedSec)));
    if (late > _delay) _delay = late;
  }

  /// 경로선 진행 alongM 에 해당하는 계획 경과초. 정차 사이는 거리로 비례 배분한다. 마지막 정차를 지났으면 남은
  /// 계획을 알 수 없어 null(그 뒤로는 셀 정차도 없다).
  int? _plannedSec(double alongM) {
    // 중간 정차 앞뒤 passedM 안에서는 재지 않는다. offsetSec 은 도착 시각이고 출발 시각은 받아오지 않아, 계획대로
    // 서 있는 시간(1~9호선 시각표 기준 20~30초)이 그대로 지연으로 잡힌다. 구간 출발역은 _alongStop 에 없어 예외다 —
    // 출발 시각을 알기 때문에 승강장에서 기다린 시간은 그대로 재야 한다.
    for (final along in _alongStop) {
      if ((along - alongM).abs() <= passedM) return null;
    }
    var prevAlong = 0.0, prevSec = 0;
    for (var i = 0; i < stops.length; i++) {
      final sec = stops[i].offsetSec;
      if (sec <= 0) continue; // 시각을 모르는 정차는 보간에서 건너뛴다(그 정차 30m 안은 위에서 이미 걸렀다)
      final along = _alongStop[i];
      if (alongM < along) {
        final span = along - prevAlong;
        if (span <= 0) return prevSec;
        return prevSec + ((sec - prevSec) * (alongM - prevAlong) / span).round();
      }
      prevAlong = along;
      prevSec = sec;
    }
    return null;
  }

  /// 다음 정차 이름. 남은 정거장이 하나면(다음이 하차역) null.
  String? nextStopName() {
    if (_remaining <= 1 || stops.isEmpty) return null;
    final i = stops.length - (_remaining - 1);
    return i >= 0 && i < stops.length ? stops[i].name : null;
  }

  /// 쓸 만한 위치면 경로선 위의 진행 거리, 아니면 null.
  double? _alongHere(double? lat, double? lon, double accuracyM) {
    if (lat == null || lon == null || accuracyM > maxAccuracyM || _points.length < 2) return null;
    final here = projectOnPolyline(lat, lon, _points);
    return here.distM > onRouteM ? null : here.alongM;
  }

  int _countAfter(double alongM) {
    var left = 1; // 하차역
    for (final along in _alongStop) {
      if (along > alongM + passedM) left++;
    }
    return left;
  }

  int? _bySchedule(DateTime now) {
    final start = _legStart;
    if (start == null) return null;
    var left = 1;
    for (final s in stops) {
      if (s.offsetSec > 0 && start.add(Duration(seconds: s.offsetSec) + _delay).isAfter(now)) left++;
    }
    return left;
  }
}
