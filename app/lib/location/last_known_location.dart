import 'package:geolocator/geolocator.dart';

/// 장소 검색이 가까운 곳·거리에 쓰는 사용자 위치. 권한이 이미 있을 때만 읽고 새로 묻지 않는다.
/// 기기에 남은 마지막 위치가 없으면 새로 받되(10초 제한), 못 받거나 오류면 null.
Future<({double lat, double lon})?> lastKnownLocation() async {
  try {
    final perm = await Geolocator.checkPermission();
    if (perm == LocationPermission.denied || perm == LocationPermission.deniedForever) return null;
    final p = await Geolocator.getLastKnownPosition() ??
        await Geolocator.getCurrentPosition(
          locationSettings: const LocationSettings(accuracy: LocationAccuracy.medium, timeLimit: Duration(seconds: 10)),
        );
    return (lat: p.latitude, lon: p.longitude);
  } catch (_) {
    return null;
  }
}
