import 'package:latlong2/latlong.dart';

import '../models/itinerary.dart';
import '../util/polyline.dart';
import 'geo.dart' as geo;

/// 안내 중 "지금 어느 구간인가"를 정한다. 위치가 현재 구간 끝점 반경 안에 들어오거나 뒤 구간 경로선을 따라가기
/// 시작하면 그 구간으로 넘기고, 위치를 믿을 수 없는 지하에서는 시간표로 넘긴다. 사용자가 손으로 앞뒤로 옮길 수도 있다.
/// 화면은 이 객체에서 현재 구간(index·mode·경로선)과 밀린 시간(shift)을 읽고, mode 를 업로드 샘플에 싣는다.
class LegTracker {
  /// legs 는 복사해 갖는다 — 경로 이탈 재탐색이 호출자의 Itinerary 를 바꾸지 않게.
  LegTracker(List<Leg> legs, {DateTime? now})
      : legs = List.of(legs),
        points = _decode(legs) {
    _enter(now ?? DateTime.now());
  }

  /// 디스크에 남겨 둔 안내를 이어받는다(앱이 죽었다 다시 켜진 경우). 구간과 밀린 시간은 저장된 값을 그대로 쓴다 —
  /// _enter 로 다시 재면 이미 지나온 구간의 계획 출발과 지금 시각 차이만큼 잘못 밀린다.
  LegTracker.resume(List<Leg> legs, {required int index, required this.shift})
      : legs = List.of(legs),
        points = _decode(legs),
        index = index < 0 ? 0 : (index >= legs.length ? legs.length - 1 : index);

  static List<List<LatLng>> _decode(List<Leg> legs) => [
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
  //   onRouteM 25 = 뒤 구간 경로선 위로 본다. 도심 GPS 오차 상한 근처.
  //   handoffAlongM 40 = 그 경로선을 이만큼 따라갔을 때만. 두 구간이 맞닿은 지점(현재 끝=다음 시작)은 제외된다.
  //   offRouteM 60 = 현재 구간 경로선에서 이만큼 벗어났을 때만(지상 구간끼리 나란한 길 오판 방지).
  //   maxAccuracyM 50 = 실내에서 튀는 표본 차단.
  //   handoffHits 2 = 연속 두 번 맞아야 넘긴다.
  static const onRouteM = 25.0;
  static const handoffAlongM = 40.0;
  static const offRouteM = 60.0;
  static const maxAccuracyM = 50.0;
  static const handoffHits = 2;
  // 시간표 인계: 믿을 만한 위치가 이만큼 끊기면 지하로 보고 시간표로 구간을 넘긴다. 2026-09-18 실기기 궤적에서
  // 지하철 구간 표본 231개 중 92개가 정확도 50m 밖이었고, 그동안 구간이 31분 넘게 첫 열차에 머물렀다.
  static const blindAfter = Duration(seconds: 20);

  int _hits = 0;
  int _hitLeg = -1;
  DateTime? _lastGoodFix;

  /// 계획 대비 밀린 시간. 구간에 늦게 들어왔으면(놓친 열차) 그만큼 뒤 구간의 예상 시각을 민다.
  Duration shift = Duration.zero;

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
  bool update(double lat, double lon, {double accuracyM = 0, DateTime? now}) {
    final t = now ?? DateTime.now();
    if (isLast) return false;
    final good = accuracyM <= maxAccuracyM;
    if (good) _lastGoodFix = t;
    if (good && geo.distanceM(lat, lon, current.toLat, current.toLon) <= arriveRadiusM) {
      _goto(index + 1, t);
      return true;
    }
    final target = good ? _legOnRoute(lat, lon) : -1;
    if (target < 0) {
      _hits = 0;
      return tick(t);
    }
    _hits = target == _hitLeg ? _hits + 1 : 1;
    _hitLeg = target;
    if (_hits < handoffHits) return tick(t);
    _goto(target, t);
    return true;
  }

  /// 위치 없이 시간만 흘렀을 때도 부른다(지하에서는 표본이 아예 끊기기도 한다). 현재 구간이 지하일 수 있는 구간
  /// (대중교통·역 안 환승 통로)이고 믿을 만한 위치가 끊긴 상태에서 예상 종료 시각이 지났으면 다음 구간으로 넘긴다.
  bool tick(DateTime now) {
    if (isLast || !(current.transitLeg || current.inStation)) return false;
    final last = _lastGoodFix;
    if (last != null && now.difference(last) < blindAfter) return false;
    final end = DateTime.tryParse(current.end);
    if (end == null || now.isBefore(end.add(shift))) return false;
    _goto(index + 1, now);
    return true;
  }

  /// 위치가 뒤 구간 경로선 위에 있으면 그 구간 번호, 아니면 -1. 바로 다음 구간은 수단을 가리지 않는다. 그보다 뒤는
  /// 현재 구간이 지하일 수 있는 구간(대중교통·역 안 환승 통로)일 때만, 지상 구간(도보·자전거)에 한해 본다 —
  /// 지하에서 구간이 밀려 있다가 지상으로 나왔을 때 한 번에 따라잡는다.
  int _legOnRoute(double lat, double lon) {
    final offCurrent =
        current.transitLeg || geo.projectOnPolyline(lat, lon, currentPoints).distM >= offRouteM;
    if (!offCurrent) return -1;
    final mayLag = current.transitLeg || current.inStation;
    for (var k = index + 1; k < legs.length; k++) {
      if (k > index + 1 && (!mayLag || legs[k].transitLeg)) continue;
      final p = geo.projectOnPolyline(lat, lon, points[k]);
      if (p.distM <= onRouteM && p.alongM >= handoffAlongM) return k;
    }
    return -1;
  }

  void _goto(int k, DateTime now, {bool manual = false}) {
    index = k;
    _hits = 0;
    _hitLeg = -1;
    _enter(now, riding: manual);
  }

  /// 구간에 들어온 시각으로 밀린 시간을 다시 잰다. 일찍 들어왔으면 계획대로다. 대중교통 구간에 자동으로 들어왔으면
  /// (승강장·정류장 도착) 들어온 뒤 처음 떠나는 차를 탄다고 보고, 사용자가 손으로 옮겼으면(riding) 이미 타고 있다고 보고
  /// 가장 최근에 떠난 차를 기준으로 한다 — 지하에서 화면이 늦게 따라와 누르는 경우다. 차 시각을 모르면 늦은 만큼 민다.
  void _enter(DateTime now, {bool riding = false}) {
    final start = DateTime.tryParse(current.start);
    if (start == null || !now.isAfter(start)) {
      shift = Duration.zero;
      return;
    }
    shift = now.difference(start);
    if (!current.transitLeg) return;
    DateTime? board = riding ? start : null;
    for (final d in current.nextDepartures) {
      final t = DateTime.tryParse(d);
      if (t == null) continue;
      if (riding) {
        if (!t.isAfter(now) && t.isAfter(board!)) board = t; // 이미 떠난 차 중 가장 늦은 것
      } else if (!t.isBefore(now) && (board == null || t.isBefore(board))) {
        board = t; // 아직 안 떠난 차 중 가장 이른 것
      }
    }
    if (board == null) return;
    final end = DateTime.tryParse(current.end);
    // 손으로 되돌린 구간의 예상 종료가 이미 지났으면 시간표 인계가 곧바로 다시 넘겨 버린다. 그때는 늦은 만큼 민다.
    if (riding && end != null && !end.add(board.difference(start)).isAfter(now)) return;
    shift = board.difference(start);
  }

  void next({DateTime? now}) {
    if (!isLast) _goto(index + 1, now ?? DateTime.now(), manual: true);
  }

  void prev({DateTime? now}) {
    if (index > 0) _goto(index - 1, now ?? DateTime.now(), manual: true);
  }

  /// 현재 구간을 다시 찾은 구간들로 갈아 끼운다(경로 이탈 재탐색). index 는 첫 새 구간을 가리킨 채로 둔다.
  /// 밀린 시간은 "새 구간들이 끝나는 시각 − 갈아 끼운 구간의 계획 종료" 로 다시 잡되 줄이지는 않는다.
  void replaceCurrent(List<Leg> fresh, DateTime now) {
    if (fresh.isEmpty) return;
    final plannedEnd = DateTime.tryParse(current.end);
    legs.replaceRange(index, index + 1, fresh);
    points.replaceRange(index, index + 1, _decode(fresh));
    _hits = 0;
    _hitLeg = -1;
    _lastGoodFix = now;
    if (plannedEnd == null) return;
    var sec = 0.0;
    for (final l in fresh) {
      sec += l.durationSec;
    }
    final late = now.add(Duration(seconds: sec.round())).difference(plannedEnd);
    if (late > shift) shift = late;
  }
}
