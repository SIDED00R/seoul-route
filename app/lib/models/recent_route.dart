import 'place.dart';
import 'plan_request.dart';

/// 서버에 쌓인 지난 검색 하나(GET /routes/recent). 저장해 둔 요청 그대로라 골라서 같은 검색을 다시 할 수 있다.
class RecentRoute {
  const RecentRoute({required this.request, required this.searchedAt});

  factory RecentRoute.fromJson(Map<String, dynamic> j) {
    final req = (j['request'] as Map<String, dynamic>?) ?? const {};
    Place place(Object? v) {
      final m = (v as Map<String, dynamic>?) ?? const {};
      return Place(
        name: (m['name'] as String?) ?? '',
        address: '',
        lat: (m['lat'] as num?)?.toDouble() ?? 0,
        lon: (m['lon'] as num?)?.toDouble() ?? 0,
      );
    }

    final via = ((req['via'] as List<dynamic>?) ?? const []).map(place).toList();
    final modes = ((req['segment_modes'] as List<dynamic>?) ?? const [])
        .map((m) => SegmentMode.values.firstWhere((v) => v.value == m, orElse: () => SegmentMode.any))
        .toList();
    return RecentRoute(
      request: PlanRequest(
        origin: place(req['origin']),
        destination: place(req['destination']),
        via: via,
        // 서버는 전 구간 any 면 segment_modes 를 싣지 않는다 — 그때는 구간 수에 맞춰 any 로 채운다.
        segmentModes: modes.isEmpty ? List.filled(via.length + 1, SegmentMode.any) : modes,
      ),
      searchedAt: DateTime.tryParse((j['searched_at'] as String?) ?? ''),
    );
  }

  final PlanRequest request;
  final DateTime? searchedAt;

  /// "서울역 → 강남역". 이름이 없으면 좌표로 대신한다.
  String get label => '${_name(request.origin)} → ${_name(request.destination)}';

  /// "경유 2곳 · 구간 수단 고정". 없으면 빈 문자열.
  String get note {
    final parts = <String>[];
    if (request.via.isNotEmpty) parts.add('경유 ${request.via.length}곳');
    if (request.segmentModes.any((m) => m != SegmentMode.any)) parts.add('구간 수단 고정');
    return parts.join(' · ');
  }

  static String _name(Place p) =>
      p.name.isNotEmpty ? p.name : '${p.lat.toStringAsFixed(4)}, ${p.lon.toStringAsFixed(4)}';
}
