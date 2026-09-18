import 'dart:math';

import 'package:latlong2/latlong.dart';

/// 두 좌표 사이 거리(m, 하버사인).
double distanceM(double lat1, double lon1, double lat2, double lon2) {
  const r = 6371000.0;
  final p1 = lat1 * pi / 180, p2 = lat2 * pi / 180;
  final dp = (lat2 - lat1) * pi / 180, dl = (lon2 - lon1) * pi / 180;
  final a = sin(dp / 2) * sin(dp / 2) + cos(p1) * cos(p2) * sin(dl / 2) * sin(dl / 2);
  return 2 * r * asin(sqrt(a));
}

/// 점을 경로선(폴리라인)에 내린 결과. distM 은 경로선까지 거리, alongM 은 경로선 시작부터 그 지점까지의 길이.
class Projection {
  const Projection({required this.distM, required this.alongM});

  final double distM;
  final double alongM;
}

/// 점을 경로선의 각 선분에 투영해 가장 가까운 지점을 찾는다. 서울 범위(위도 37도대)에서만 쓰므로 위경도를
/// 미터 평면으로 근사한다(위도 1도 = 111,195m, 경도는 cos(위도) 배).
Projection projectOnPolyline(double lat, double lon, List<LatLng> points) {
  if (points.isEmpty) return const Projection(distM: double.infinity, alongM: 0);
  const mPerDeg = 111195.0;
  final kx = mPerDeg * cos(lat * pi / 180), ky = mPerDeg;
  double x(LatLng p) => p.longitude * kx;
  double y(LatLng p) => p.latitude * ky;
  final px = lon * kx, py = lat * ky;
  if (points.length == 1) {
    final dx = px - x(points[0]), dy = py - y(points[0]);
    return Projection(distM: sqrt(dx * dx + dy * dy), alongM: 0);
  }
  var best = double.infinity, bestAlong = 0.0, along = 0.0;
  for (var i = 0; i + 1 < points.length; i++) {
    final ax = x(points[i]), ay = y(points[i]);
    final bx = x(points[i + 1]), by = y(points[i + 1]);
    final vx = bx - ax, vy = by - ay;
    final segLen = sqrt(vx * vx + vy * vy);
    var t = 0.0;
    if (segLen > 0) {
      t = ((px - ax) * vx + (py - ay) * vy) / (segLen * segLen);
      t = t.clamp(0.0, 1.0);
    }
    final dx = px - (ax + vx * t), dy = py - (ay + vy * t);
    final d = sqrt(dx * dx + dy * dy);
    if (d < best) {
      best = d;
      bestAlong = along + segLen * t;
    }
    along += segLen;
  }
  return Projection(distM: best, alongM: bestAlong);
}
