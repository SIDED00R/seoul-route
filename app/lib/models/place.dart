// 장소 검색 결과 한 건. 백엔드 /places/search 응답과 1:1.
class Place {
  const Place({
    required this.name,
    required this.address,
    required this.lat,
    required this.lon,
    this.category = '',
    this.distanceM,
  });

  final String name;
  final String address;
  final String category;
  final double lat;
  final double lon;

  /// 검색 요청에 보낸 위치에서 직선거리(m). 위치 없이 검색했으면 null.
  final int? distanceM;

  factory Place.fromJson(Map<String, dynamic> j) => Place(
        name: j['name'] as String,
        address: (j['address'] as String?) ?? '',
        category: (j['category'] as String?) ?? '',
        lat: (j['lat'] as num).toDouble(),
        lon: (j['lon'] as num).toDouble(),
        distanceM: (j['distance_m'] as num?)?.toInt(),
      );
}
