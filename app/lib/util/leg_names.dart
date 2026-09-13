import '../models/plan_request.dart';

/// OTP 는 출발·도착을 "Origin"/"Destination" 으로 돌려준다. 구간 분할 경로에서는 경유지도 그렇게 오므로
/// 좌표가 가장 가까운 지점(출발·경유·도착)의 이름으로 바꾼다. 상세·안내 화면이 같이 쓴다.
String legEndpointName(PlanRequest request, String n, double lat, double lon) {
  if (n != 'Origin' && n != 'Destination') return n;
  final candidates = [request.origin, ...request.via, request.destination];
  var best = candidates.first;
  var bestD = double.infinity;
  for (final p in candidates) {
    final d = (p.lat - lat) * (p.lat - lat) + (p.lon - lon) * (p.lon - lon);
    if (d < bestD) {
      bestD = d;
      best = p;
    }
  }
  return best.name;
}
