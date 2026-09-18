import 'package:latlong2/latlong.dart';

/// 좌표 목록을 Google encoded polyline(1e-5)으로 만든다. 서버 응답을 흉내 내는 테스트 전용 도우미다
/// (앱은 디코딩만 한다).
String encodePolyline(List<LatLng> points) {
  final buf = StringBuffer();
  var lat = 0, lon = 0;
  for (final p in points) {
    final la = (p.latitude * 1e5).round(), lo = (p.longitude * 1e5).round();
    _chunk(buf, la - lat);
    _chunk(buf, lo - lon);
    lat = la;
    lon = lo;
  }
  return buf.toString();
}

void _chunk(StringBuffer buf, int v) {
  var x = v < 0 ? ~(v << 1) : (v << 1);
  while (x >= 0x20) {
    buf.writeCharCode((0x20 | (x & 0x1f)) + 63);
    x >>= 5;
  }
  buf.writeCharCode(x + 63);
}
