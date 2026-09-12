// 백엔드 /routes/plan 응답 모델. 필드명은 backend/internal/otp/client.go 의 JSON 태그와 같다.
class Leg {
  const Leg({
    required this.mode,
    required this.durationSec,
    required this.distanceM,
    required this.fromName,
    required this.toName,
    required this.fromLat,
    required this.fromLon,
    required this.toLat,
    required this.toLon,
    required this.route,
    required this.rentedBike,
    required this.transitLeg,
    required this.polyline,
    required this.start,
    required this.end,
    this.prevDepartures = const [],
    this.nextDepartures = const [],
    this.headwaySec = 0,
    this.realtimeArrivalsSec = const [],
  });

  final String mode; // WALK / BICYCLE / BUS / SUBWAY ...
  final double durationSec;
  final double distanceM;
  final String fromName;
  final String toName;
  final double fromLat;
  final double fromLon;
  final double toLat;
  final double toLon;
  final String route; // 버스 번호·호선
  final bool rentedBike; // 따릉이 leg
  final bool transitLeg;
  final String polyline; // Google encoded polyline
  final String start; // RFC3339
  final String end;
  final List<String> prevDepartures; // 같은 구간 이전 차 출발(RFC3339, 지하철)
  final List<String> nextDepartures; // 다음 차 출발(RFC3339, 지하철)
  final int headwaySec; // 배차간격(버스, 생성 GTFS)
  final List<int> realtimeArrivalsSec; // 첫 탑승 정류장의 실시간 다음 차(초)

  factory Leg.fromJson(Map<String, dynamic> j) => Leg(
        mode: j['mode'] as String,
        durationSec: (j['duration_sec'] as num).toDouble(),
        distanceM: (j['distance_m'] as num).toDouble(),
        fromName: (j['from_name'] as String?) ?? '',
        toName: (j['to_name'] as String?) ?? '',
        fromLat: (j['from_lat'] as num).toDouble(),
        fromLon: (j['from_lon'] as num).toDouble(),
        toLat: (j['to_lat'] as num).toDouble(),
        toLon: (j['to_lon'] as num).toDouble(),
        route: (j['route'] as String?) ?? '',
        rentedBike: (j['rented_bike'] as bool?) ?? false,
        transitLeg: (j['transit_leg'] as bool?) ?? false,
        polyline: (j['polyline'] as String?) ?? '',
        start: (j['start'] as String?) ?? '',
        end: (j['end'] as String?) ?? '',
        prevDepartures: ((j['prev_departures'] as List<dynamic>?) ?? const []).cast<String>(),
        nextDepartures: ((j['next_departures'] as List<dynamic>?) ?? const []).cast<String>(),
        headwaySec: (j['headway_sec'] as num?)?.toInt() ?? 0,
        realtimeArrivalsSec: ((j['realtime_arrivals_sec'] as List<dynamic>?) ?? const [])
            .map((e) => (e as num).toInt())
            .toList(),
      );

  /// 앞뒤 차 안내 한 줄. 실시간 > 시간표 앞뒤 차 > 배차간격 순으로 있는 것만 보여준다. 없으면 null.
  String? get scheduleLabel {
    final parts = <String>[];
    if (realtimeArrivalsSec.isNotEmpty) {
      parts.add('실시간 다음 차 ${realtimeArrivalsSec.map((s) => '${(s / 60).round()}분').join(', ')} 후');
    }
    String hhmm(String rfc) => rfc.length >= 16 ? rfc.substring(11, 16) : rfc;
    if (prevDepartures.isNotEmpty) parts.add('앞차 ${prevDepartures.map(hhmm).join(', ')}');
    if (nextDepartures.isNotEmpty) parts.add('다음 ${nextDepartures.map(hhmm).join(', ')}');
    if (headwaySec > 0) parts.add('배차 약 ${(headwaySec / 60).round()}분');
    return parts.isEmpty ? null : parts.join(' · ');
  }

  /// 화면에 보여줄 수단 이름. 따릉이는 BICYCLE 에 rentedBike 가 붙는다.
  String get label {
    switch (mode) {
      case 'WALK':
        return '도보';
      case 'BICYCLE':
        return rentedBike ? '따릉이' : '자전거';
      case 'BUS':
        return route.isEmpty ? '버스' : '버스 $route';
      case 'SUBWAY':
      case 'RAIL':
        return route.isEmpty ? '지하철' : route;
      default:
        return mode;
    }
  }
}

class Itinerary {
  const Itinerary({
    required this.start,
    required this.end,
    required this.durationSec,
    required this.transfers,
    required this.walkM,
    required this.legs,
    this.realtime = false,
    this.realtimeDeltaSec = 0,
    this.departInSec = 0,
  });

  final String start;
  final String end;
  final double durationSec;
  final int transfers;
  final double walkM;
  final List<Leg> legs;
  final bool realtime; // 첫 탑승 대기가 실시간 도착정보로 보정됨
  final double realtimeDeltaSec; // 시간표 대비 보정(초, 음수면 시간표보다 빠름)
  final double departInSec; // 지금 출발 요청에서 출발까지 기다리는 초. 총 소요 = departIn + duration

  factory Itinerary.fromJson(Map<String, dynamic> j) => Itinerary(
        start: (j['start'] as String?) ?? '',
        end: (j['end'] as String?) ?? '',
        durationSec: (j['duration_sec'] as num).toDouble(),
        transfers: (j['transfers'] as num?)?.toInt() ?? 0,
        walkM: (j['walk_distance_m'] as num?)?.toDouble() ?? 0,
        legs: ((j['legs'] as List<dynamic>?) ?? const [])
            .map((e) => Leg.fromJson(e as Map<String, dynamic>))
            .toList(),
        realtime: (j['realtime'] as bool?) ?? false,
        realtimeDeltaSec: (j['realtime_delta_sec'] as num?)?.toDouble() ?? 0,
        departInSec: (j['depart_in_sec'] as num?)?.toDouble() ?? 0,
      );

  /// 지금부터 도착까지(출발 대기 포함). 카카오맵의 "총 소요"와 같은 기준.
  int get minutes => ((departInSec + durationSec) / 60).round();

  /// "3분 후 출발" 문구. 1분 미만이면 null.
  String? get departLabel {
    final m = (departInSec / 60).round();
    return m >= 1 ? '$m분 후 출발' : null;
  }

  /// "실시간 −3분" 같은 배지 문구. 보정이 없으면 null.
  String? get realtimeLabel {
    if (!realtime) return null;
    final m = (realtimeDeltaSec / 60).round();
    if (m == 0) return '실시간';
    return '실시간 ${m > 0 ? '+' : '−'}${m.abs()}분';
  }
}

class PlanResult {
  const PlanResult({required this.itineraries, this.reason, this.note});

  final List<Itinerary> itineraries;
  final String? reason; // 경로 없음 사유(있으면 itineraries 는 빈 목록)
  final String? note;

  factory PlanResult.fromJson(Map<String, dynamic> j) => PlanResult(
        itineraries: ((j['itineraries'] as List<dynamic>?) ?? const [])
            .map((e) => Itinerary.fromJson(e as Map<String, dynamic>))
            .toList(),
        reason: j['reason'] as String?,
        note: j['note'] as String?,
      );
}
