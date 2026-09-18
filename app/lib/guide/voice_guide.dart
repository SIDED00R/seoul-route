/// 문장 하나를 읽는 함수. 실제 발화는 tts_speaker.dart 가, 테스트는 기록용 함수가 맡는다.
typedef Speak = Future<void> Function(String text);

/// 안내 문장을 읽되 같은 안내 시점(cueKey)은 한 번만 읽는다. 위치는 몇 초마다 들어오고 문장의 거리·정거장 수는 그때마다
/// 달라지므로, 문장이 아니라 시점 이름으로 되풀이를 막는다.
class VoiceGuide {
  VoiceGuide({this.speak, this.enabled = true});

  Speak? speak;
  bool enabled;
  final Set<String> _spoken = {};

  Future<void> say(String? text, {required String cueKey}) async {
    final f = speak;
    if (!enabled || f == null || text == null || text.isEmpty || !_spoken.add(cueKey)) return;
    await f(text);
  }

  /// 사용자가 구간을 손으로 옮겼을 때 그 구간의 안내를 다시 읽을 수 있게 한다.
  void forget(String prefix) => _spoken.removeWhere((k) => k.startsWith(prefix));
}
