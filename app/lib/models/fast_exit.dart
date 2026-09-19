/// 지하철 하차역의 설비 하나(서버 leg.fast_exit[]): 계단·에스컬레이터·엘리베이터와 그 앞 칸-문("3-3") 목록.
class FastExitFacility {
  const FastExitFacility({required this.name, required this.doors});

  factory FastExitFacility.fromJson(Map<String, dynamic> j) => FastExitFacility(
        name: j['facility'] as String? ?? '',
        doors: ((j['doors'] as List<dynamic>?) ?? const []).map((d) => d as String).toList(),
      );

  final String name;
  final List<String> doors;
}
