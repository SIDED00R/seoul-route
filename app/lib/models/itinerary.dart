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
      );

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
  });

  final String start;
  final String end;
  final double durationSec;
  final int transfers;
  final double walkM;
  final List<Leg> legs;

  factory Itinerary.fromJson(Map<String, dynamic> j) => Itinerary(
        start: (j['start'] as String?) ?? '',
        end: (j['end'] as String?) ?? '',
        durationSec: (j['duration_sec'] as num).toDouble(),
        transfers: (j['transfers'] as num?)?.toInt() ?? 0,
        walkM: (j['walk_distance_m'] as num?)?.toDouble() ?? 0,
        legs: ((j['legs'] as List<dynamic>?) ?? const [])
            .map((e) => Leg.fromJson(e as Map<String, dynamic>))
            .toList(),
      );

  int get minutes => (durationSec / 60).round();
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
