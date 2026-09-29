import 'package:flutter_tts/flutter_tts.dart';

import 'audio_focus.dart';
import 'speech_focus.dart';
import 'voice_guide.dart';

/// 폰 내장 음성 합성으로 안내를 읽는다. 엔진이 없거나 준비에 실패하면 null 을 돌려주고 안내는 소리 없이 진행한다.
/// 읽는 동안 다른 앱 소리를 줄이는 오디오 포커스는 SpeechFocus 가 남은 발화 수로 잡고 놓는다.
class TtsSpeaker {
  TtsSpeaker._(this._tts) {
    _tts.setCompletionHandler(_focus.ended);
    _tts.setCancelHandler(_focus.ended);
    _tts.setErrorHandler((_) => _focus.ended());
  }

  final FlutterTts _tts;
  final _focus = SpeechFocus(duck: AudioFocus.duck, release: AudioFocus.release);

  // 삼성 TTS 는 2025-06 업데이트부터 다른 앱에서 쓸 수 없다. 구글 엔진이 있으면 그걸 쓴다.
  static const googleEngine = 'com.google.android.tts';

  static Future<TtsSpeaker?> create() async {
    try {
      final tts = FlutterTts();
      final engines = (await tts.getEngines) as List<dynamic>? ?? const [];
      if (engines.contains(googleEngine)) await tts.setEngine(googleEngine);
      if (await tts.isLanguageAvailable('ko-KR') != true) return null;
      await tts.setLanguage('ko-KR');
      await tts.setQueueMode(1); // 이어서 읽기 — 앞 문장을 자르지 않는다
      await tts.setAudioAttributesForNavigation(); // 발화를 내비게이션 안내 음성으로 낸다
      return TtsSpeaker._(tts);
    } catch (_) {
      return null; // 플러그인·엔진 없음
    }
  }

  Speak get speak => (text) async {
        _focus.began(); // began 은 기다리지 않는다 — _tts.speak 가 부른 순서대로 큐에 들어간다
        try {
          if (await _tts.speak(text) != 1) _focus.ended(); // 큐에 넣지 못하면 끝 콜백이 오지 않는다
        } catch (_) {
          _focus.ended(); // 한 문장을 못 읽어도 안내는 이어간다
        }
      };

  Future<void> stop() async {
    try {
      await _tts.stop();
    } catch (_) {
      // 종료 중이라 무시
    }
    _focus.reset();
  }
}
