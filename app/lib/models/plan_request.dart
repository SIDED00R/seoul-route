import 'place.dart';

// 구간별 수단 고정. 값은 backend/internal/route/plan.go 의 SegmentMode 와 같다.
enum SegmentMode {
  any('any', '전체'),
  walk('walk', '도보'),
  bike('bike', '따릉이'),
  transit('transit', '대중교통');

  const SegmentMode(this.value, this.label);
  final String value;
  final String label;
}

class PlanRequest {
  const PlanRequest({
    required this.origin,
    required this.destination,
    this.via = const [],
    this.segmentModes = const [],
  });

  final Place origin;
  final Place destination;
  final List<Place> via; // 최대 5개(서버 검증)
  final List<SegmentMode> segmentModes; // 길이 = via 수 + 1. 비면 전 구간 any

  Map<String, dynamic> toJson() => {
        'origin': {'lat': origin.lat, 'lon': origin.lon},
        'destination': {'lat': destination.lat, 'lon': destination.lon},
        if (via.isNotEmpty)
          'via': via.map((p) => {'lat': p.lat, 'lon': p.lon}).toList(),
        if (segmentModes.any((m) => m != SegmentMode.any))
          'segment_modes': segmentModes.map((m) => m.value).toList(),
      };
}
