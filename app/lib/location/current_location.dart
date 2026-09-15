import 'dart:async';

import 'package:geolocator/geolocator.dart';

import '../models/place.dart';

/// 현재 위치를 못 받은 이유(화면에 그대로 보여 준다).
class LocationException implements Exception {
  const LocationException(this.message);

  final String message;

  @override
  String toString() => message;
}

/// 폰의 현재 위치를 출발지로 쓸 Place 로 만든다. 이름은 "현재 위치"(서버 역 앵커링 대상이 아니다),
/// 주소 자리에는 좌표 대신 정확도만 둔다(화면·스크린샷에 집 좌표를 드러내지 않는다).
Future<Place> currentPlace() async {
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
    return Place(name: '현재 위치', address: '정확도 약 ${p.accuracy.round()}m', lat: p.latitude, lon: p.longitude);
  } on TimeoutException {
    throw const LocationException('15초 안에 위치를 받지 못했습니다. 잠시 뒤 다시 누르세요.');
  }
}
