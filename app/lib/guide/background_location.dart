import 'package:flutter/services.dart';
import 'package:geolocator/geolocator.dart';

/// 안내 중 위치 스트림 설정. 포그라운드 서비스(상단 "안내 중" 알림)로 받아 화면이 꺼지거나 다른 앱으로 넘어가도 샘플이
/// 이어진다. 스트림 구독을 취소하면 서비스와 알림이 같이 내려간다.
LocationSettings guideLocationSettings(Duration interval) => AndroidSettings(
      accuracy: LocationAccuracy.high,
      distanceFilter: 0,
      intervalDuration: interval,
      foregroundNotificationConfig: const ForegroundNotificationConfig(
        notificationTitle: '안내 중',
        notificationText: '위치를 기록하고 있습니다. 안내를 종료하면 멈춥니다.',
        notificationChannelName: '안내 위치 기록',
        enableWakeLock: true, // 서비스가 도는 동안 partial wake lock
        setOngoing: true,
      ),
    );

const _notificationChannel = MethodChannel('seoul_route/notification_permission');

/// 안내 알림을 알림창에 띄우는 권한(Android 13+ POST_NOTIFICATIONS)을 요청한다. 허용이면 true, 채널이 없으면 false.
Future<bool> requestNotificationPermission() async {
  try {
    return await _notificationChannel.invokeMethod<bool>('request') ?? false;
  } on MissingPluginException {
    return false;
  } on PlatformException {
    return false;
  }
}
