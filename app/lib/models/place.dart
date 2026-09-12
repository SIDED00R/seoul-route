// 장소 검색 결과 한 건. 백엔드 /places/search 응답과 1:1.
class Place {
  const Place({
    required this.name,
    required this.address,
    required this.lat,
    required this.lon,
    this.category = '',
  });

  final String name;
  final String address;
  final String category;
  final double lat;
  final double lon;

  factory Place.fromJson(Map<String, dynamic> j) => Place(
        name: j['name'] as String,
        address: (j['address'] as String?) ?? '',
        category: (j['category'] as String?) ?? '',
        lat: (j['lat'] as num).toDouble(),
        lon: (j['lon'] as num).toDouble(),
      );
}
