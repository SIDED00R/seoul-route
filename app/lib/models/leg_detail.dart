/// 도보·자전거 구간의 안내 단계 하나(서버 leg.steps[]). dir 은 이 단계 시작점에서의 회전(OTP relativeDirection),
/// distanceM 은 그 뒤로 가는 거리. street 는 이름 있는 길에만 있고, entrance 는 역 출입구를 지나는 단계에만 있다.
class WalkStep {
  const WalkStep({
    required this.dir,
    this.abs = '',
    this.street = '',
    required this.distanceM,
    required this.lat,
    required this.lon,
    this.exit = '',
    this.entrance = '',
  });

  factory WalkStep.fromJson(Map<String, dynamic> j) => WalkStep(
        dir: j['dir'] as String? ?? '',
        abs: j['abs'] as String? ?? '',
        street: j['street'] as String? ?? '',
        distanceM: (j['distance_m'] as num?)?.toDouble() ?? 0,
        lat: (j['lat'] as num?)?.toDouble() ?? 0,
        lon: (j['lon'] as num?)?.toDouble() ?? 0,
        exit: j['exit'] as String? ?? '',
        entrance: j['entrance'] as String? ?? '',
      );

  final String dir;
  final String abs;
  final String street;
  final double distanceM;
  final double lat;
  final double lon;
  final String exit;
  final String entrance;
}

/// 대중교통 구간의 중간 정차 하나(서버 leg.stops[]). offsetSec 은 구간 출발부터 이 정차 도착까지의 초다.
class TransitStop {
  const TransitStop({
    required this.name,
    required this.lat,
    required this.lon,
    this.stopId = '',
    this.offsetSec = 0,
  });

  factory TransitStop.fromJson(Map<String, dynamic> j) => TransitStop(
        name: j['name'] as String? ?? '',
        lat: (j['lat'] as num?)?.toDouble() ?? 0,
        lon: (j['lon'] as num?)?.toDouble() ?? 0,
        stopId: j['stop_id'] as String? ?? '',
        offsetSec: (j['offset_sec'] as num?)?.toInt() ?? 0,
      );

  final String name;
  final double lat;
  final double lon;
  final String stopId;
  final int offsetSec;
}
