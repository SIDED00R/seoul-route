// 안내 중 위치 샘플 한 건. 백엔드 POST /trips/{id}/traces 의 samples[] 원소와 1:1.
class TraceSample {
  const TraceSample({
    required this.ts,
    required this.lat,
    required this.lon,
    required this.accuracyM,
    required this.mode,
    this.activity,
  });

  final DateTime ts;
  final double lat;
  final double lon;
  final double accuracyM;
  final String mode; // walk / bicycle / transit — 샘플 시점의 안내 구간 수단
  final String? activity; // 폰 활동 인식 판정 walk / bicycle / vehicle / still / unknown, 못 받으면 null

  Map<String, dynamic> toJson() => {
        'ts': ts.toUtc().toIso8601String(),
        'lat': lat,
        'lon': lon,
        'accuracy_m': accuracyM,
        'mode': mode,
        if (activity != null) 'activity': activity,
      };
}

/// 사용자 속도 프로파일 한 수단(GET /users/me/speed 의 walk / bicycle).
class SpeedProfile {
  const SpeedProfile({required this.speedMps, required this.nTrips, required this.priorMps});

  final double speedMps;
  final int nTrips;
  final double priorMps;

  factory SpeedProfile.fromJson(Map<String, dynamic> j) => SpeedProfile(
        speedMps: (j['speed_mps'] as num).toDouble(),
        nTrips: (j['n_trips'] as num).toInt(),
        priorMps: (j['prior_mps'] as num).toDouble(),
      );

  /// "1.35 m/s (4.9 km/h, trip 3회)" / 표본이 없으면 "기본값 1.2 m/s".
  String get label => nTrips == 0
      ? '기본값 ${priorMps.toStringAsFixed(2)} m/s'
      : '${speedMps.toStringAsFixed(2)} m/s (${(speedMps * 3.6).toStringAsFixed(1)} km/h, trip $nTrips회)';
}
