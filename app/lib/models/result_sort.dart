import 'itinerary.dart';

/// 경로 목록 정렬 기준. 추천순은 서버 순서(출발 대기 + 소요 + 환승 1회당 240초 + 따릉이 대여가 있으면 300초 점수,
/// backend route/ranking.go), 최소시간순은 화면에 보이는 분([Itinerary.minutes]) 오름차순.
enum ResultSort {
  recommended('추천순'),
  fastest('최소시간순');

  const ResultSort(this.label);
  final String label;
}

/// [sort] 기준으로 정렬한 새 목록. 최소시간순에서 표시 분이 같으면 추천순(서버 순서)을 유지한다.
List<Itinerary> sortItineraries(List<Itinerary> its, ResultSort sort) {
  if (sort == ResultSort.recommended) return its;
  final indexed = [for (var i = 0; i < its.length; i++) (i, its[i])];
  indexed.sort((a, b) {
    final byMinutes = a.$2.minutes.compareTo(b.$2.minutes);
    return byMinutes != 0 ? byMinutes : a.$1.compareTo(b.$1);
  });
  return [for (final (_, it) in indexed) it];
}
