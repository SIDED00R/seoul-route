import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/speech_focus.dart';

void main() {
  late List<String> calls;
  SpeechFocus focus({Duration maxQuiet = const Duration(seconds: 20)}) => SpeechFocus(
        duck: () async => calls.add('duck'),
        release: () async => calls.add('release'),
        maxQuiet: maxQuiet,
      );
  setUp(() => calls = []);

  test('겹친 발화는 한 번 잡고, 모두 끝나야 놓는다', () async {
    final f = focus();
    f.began();
    f.began(); // 앞 문장을 읽는 중에 다음 문장이 큐에 들어온다
    f.ended();
    expect(calls, ['duck']);
    f.ended();
    expect(calls, ['duck', 'release']);
    f.ended(); // 남은 발화가 없을 때 온 콜백은 무시
    expect(calls, ['duck', 'release']);
    f.began();
    f.ended();
    expect(calls, ['duck', 'release', 'duck', 'release']);
  });

  test('끝 콜백이 오지 않아도 마지막 소식 뒤 maxQuiet 가 지나면 놓는다', () async {
    final f = focus(maxQuiet: const Duration(milliseconds: 50));
    f.began();
    f.began();
    f.ended(); // 하나만 끝나고 하나는 콜백을 잃었다
    await Future<void>.delayed(const Duration(milliseconds: 120));
    expect(calls, ['duck', 'release']);
    expect(f.pending, 0);
  });

  test('중지하면 남은 발화를 버리고 놓는다', () async {
    final f = focus();
    f.began();
    f.began();
    f.reset();
    expect(calls, ['duck', 'release']);
    expect(f.pending, 0);
    f.reset(); // 잡은 게 없으면 놓지 않는다
    expect(calls, ['duck', 'release']);
  });
}
