import 'dart:async';

import 'package:geolocator/geolocator.dart';

import '../api/client.dart';
import '../models/place.dart';

/// 현재 위치를 못 받은 이유(화면에 그대로 보여 준다).
class LocationException implements Exception {
  const LocationException(this.message);

  final String message;

  @override
  String toString() => message;
}

/// 폰의 현재 위치를 출발지로 쓸 Place 로 만든다. 이름은 "현재 위치 · <건물 이름>"(서버 역 앵커링은 첫 낱말이
/// "…역" 일 때만 걸리므로 건물 이름이 역이어도 좌표 출발이 유지된다), 주소 자리에는 좌표 대신 도로명 주소와
/// 정확도를 둔다(화면·스크린샷에 집 좌표를 드러내지 않는다).
Future<Place> currentPlace(ApiClient api) async {
  if (!await Geolocator.isLocationServiceEnabled()) {
    throw const LocationException('기기 위치 서비스가 꺼져 있습니다.');
  }
  var perm = await Geolocator.checkPermission();
  if (perm == LocationPermission.denied) perm = await Geolocator.requestPermission();
  if (perm == LocationPermission.denied || perm == LocationPermission.deniedForever) {
    throw const LocationException('위치 권한이 없습니다. 설정에서 허용한 뒤 다시 누르세요.');
  }
  try {
    final p = await Geolocator.getCurrentPosition(
      locationSettings: const LocationSettings(accuracy: LocationAccuracy.high, timeLimit: Duration(seconds: 15)),
    );
    final accuracy = '정확도 약 ${p.accuracy.round()}m';
    // 이름 조회는 있으면 좋은 것이다. 서버 미설정(503)·오류·시간 초과면 좌표만 쓴다.
    try {
      final r = await api.reversePlace(p.latitude, p.longitude);
      if (r.name.isNotEmpty) {
        final address = r.address.isEmpty ? accuracy : '${r.address} · $accuracy';
        return Place(name: '현재 위치 · ${r.name}', address: address, lat: p.latitude, lon: p.longitude);
      }
    } catch (_) {
      // 이름 없이 진행
    }
    return Place(name: '현재 위치', address: accuracy, lat: p.latitude, lon: p.longitude);
  } on TimeoutException {
    throw const LocationException('15초 안에 위치를 받지 못했습니다. 잠시 뒤 다시 누르세요.');
  }
}
