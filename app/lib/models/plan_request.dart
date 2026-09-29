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
    this.viaStayMin = const [],
    this.bikeLimitMin = 0,
    this.depart,
    this.arrive,
  });

  final Place origin;
  final Place destination;
  final List<Place> via; // 최대 5개(서버 검증)
  final List<SegmentMode> segmentModes; // 길이 = via 수 + 1. 비면 전 구간 any
  final List<int> viaStayMin; // 경유지마다 머무는 분(0 = 바로 통과). 비거나 짧으면 나머지는 0
  final int bikeLimitMin; // 따릉이 이용권 대여 1회 한도(분). 0 이면 보내지 않는다(서버는 제한 없음)
  final DateTime? depart; // 이 시각에 출발. depart·arrive 둘 다 없으면 지금 출발
  final DateTime? arrive; // 이 시각까지 도착(서버는 경유지·구간 수단 고정이 없을 때만 받는다)

  /// i 번째 경유지 체류(분).
  int stayAt(int i) => i < viaStayMin.length ? viaStayMin[i] : 0;

  /// 경유지 체류 합(분).
  int get totalStayMin => [for (var i = 0; i < via.length; i++) stayAt(i)].fold(0, (a, b) => a + b);

  // name 은 서버가 "…역" 이면 근처 같은 이름 역(GTFS 부모역)으로 앵커링하는 데 쓴다(역사 좌표 스냅 문제).
  static Map<String, dynamic> _pt(Place p) => {'lat': p.lat, 'lon': p.lon, 'name': p.name};

  Map<String, dynamic> toJson() => {
        'origin': _pt(origin),
        'destination': _pt(destination),
        if (via.isNotEmpty)
          'via': [
            for (var i = 0; i < via.length; i++) {..._pt(via[i]), if (stayAt(i) > 0) 'stay_min': stayAt(i)},
          ],
        if (segmentModes.any((m) => m != SegmentMode.any))
          'segment_modes': segmentModes.map((m) => m.value).toList(),
        if (bikeLimitMin > 0) 'bike_limit_min': bikeLimitMin,
        // 서버(Go time.Time)는 시간대가 붙은 RFC3339 만 읽으므로 UTC("…Z")로 보낸다.
        if (depart != null) 'depart': depart!.toUtc().toIso8601String(),
        if (arrive != null) 'arrive': arrive!.toUtc().toIso8601String(),
      };

  /// toJson 을 되돌린다(디스크에 남긴 안내를 되살릴 때). _pt 가 이름·좌표만 싣기 때문에 주소는 빈 값이 된다 —
  /// 안내 문구는 이름만 쓴다. segment_modes 가 없으면 전 구간 any 로 본다(toJson 이 그때 생략한다).
  factory PlanRequest.fromJson(Map<String, dynamic> j) {
    final rawVia = (j['via'] as List<dynamic>?) ?? const [];
    final via = rawVia.map(_place).toList();
    final modes = (j['segment_modes'] as List<dynamic>?)
        ?.map((v) => SegmentMode.values.firstWhere((m) => m.value == v, orElse: () => SegmentMode.any))
        .toList();
    return PlanRequest(
      origin: _place(j['origin']),
      destination: _place(j['destination']),
      via: via,
      segmentModes: modes ?? List.filled(via.length + 1, SegmentMode.any),
      viaStayMin: [for (final v in rawVia) ((v as Map<String, dynamic>)['stay_min'] as num?)?.toInt() ?? 0],
      bikeLimitMin: (j['bike_limit_min'] as num?)?.toInt() ?? 0,
      depart: DateTime.tryParse((j['depart'] as String?) ?? '')?.toLocal(),
      arrive: DateTime.tryParse((j['arrive'] as String?) ?? '')?.toLocal(),
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
