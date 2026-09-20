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

  // name 은 서버가 "…역" 이면 근처 같은 이름 역(GTFS 부모역)으로 앵커링하는 데 쓴다(역사 좌표 스냅 문제).
  static Map<String, dynamic> _pt(Place p) => {'lat': p.lat, 'lon': p.lon, 'name': p.name};

  Map<String, dynamic> toJson() => {
        'origin': _pt(origin),
        'destination': _pt(destination),
        if (via.isNotEmpty) 'via': via.map(_pt).toList(),
        if (segmentModes.any((m) => m != SegmentMode.any))
          'segment_modes': segmentModes.map((m) => m.value).toList(),
      };

  /// toJson 을 되돌린다(디스크에 남긴 안내를 되살릴 때). _pt 가 이름·좌표만 싣기 때문에 주소는 빈 값이 된다 —
  /// 안내 문구는 이름만 쓴다. segment_modes 가 없으면 전 구간 any 로 본다(toJson 이 그때 생략한다).
  factory PlanRequest.fromJson(Map<String, dynamic> j) {
    final via = ((j['via'] as List<dynamic>?) ?? const []).map(_place).toList();
    final modes = (j['segment_modes'] as List<dynamic>?)
        ?.map((v) => SegmentMode.values.firstWhere((m) => m.value == v, orElse: () => SegmentMode.any))
        .toList();
    return PlanRequest(
      origin: _place(j['origin']),
      destination: _place(j['destination']),
      via: via,
      segmentModes: modes ?? List.filled(via.length + 1, SegmentMode.any),
    );
  }

  static Place _place(dynamic v) {
    final m = v as Map<String, dynamic>;
    return Place(
      name: (m['name'] as String?) ?? '',
      address: '',
      lat: (m['lat'] as num).toDouble(),
      lon: (m['lon'] as num).toDouble(),
    );
  }
}
