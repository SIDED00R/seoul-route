import 'package:flutter/services.dart';

/// 안내 음성이 나오는 동안 다른 앱(영상·음악) 소리를 줄이는 오디오 포커스(MainActivity GuideAudioFocus). 플랫폼이 없거나
/// 실패하면 아무것도 하지 않는다.
class AudioFocus {
  static const _channel = MethodChannel('seoul_route/audio_focus');

  static Future<void> duck() => _call('duck');
  static Future<void> release() => _call('release');

  static Future<void> _call(String method) async {
    try {
      await _channel.invokeMethod<void>(method);
    } catch (_) {
      // 포커스 없이도 읽기는 이어간다
    }
  }
}
