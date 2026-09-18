/// 문장 하나를 읽는 함수. 실제 발화는 tts_speaker.dart 가, 테스트는 기록용 함수가 맡는다.
typedef Speak = Future<void> Function(String text);

/// 안내 문장을 읽되 같은 문장을 잇따라 되풀이하지 않는다. 위치는 몇 초마다 들어오므로 단계가 바뀔 때만 읽힌다.
class VoiceGuide {
  VoiceGuide({this.speak, this.enabled = true});

  Speak? speak;
  bool enabled;
  String? _last;

  Future<void> say(String? text) async {
    final f = speak;
    if (!enabled || f == null || text == null || text.isEmpty || text == _last) return;
    _last = text;
    await f(text);
  }
}
