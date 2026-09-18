import 'package:flutter/services.dart';

/// 알림창에 띄우는 안내 진행 알림(지금 할 일·도착 예정·남은 시간). 위치 포그라운드 서비스의 알림은 문구를 바꿀 수
/// 없어서 따로 둔다. 안드로이드 쪽 구현은 MainActivity 의 같은 이름 채널이다.
class GuideStatusNotification {
  static const _channel = MethodChannel('seoul_route/guide_status');

  String? _shown;

  /// 내용이 바뀌었을 때만 알림을 갱신한다. 채널이 없거나(테스트·다른 플랫폼) 실패해도 안내는 계속한다.
  Future<void> show(String title, String text) async {
    final key = '$title|$text';
    if (key == _shown) return;
    _shown = key;
    try {
      await _channel.invokeMethod<void>('show', {'title': title, 'text': text});
    } on MissingPluginException {
      // 알림 없이 진행
    } on PlatformException {
      // 알림 없이 진행
    }
  }

  Future<void> cancel() async {
    _shown = null;
    try {
      await _channel.invokeMethod<void>('cancel');
    } on MissingPluginException {
      // 알림 없이 진행
    } on PlatformException {
      // 알림 없이 진행
    }
  }
}
