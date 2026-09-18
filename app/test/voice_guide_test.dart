import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/voice_guide.dart';

void main() {
  test('같은 안내 시점은 문장이 달라져도 한 번만 읽는다', () async {
    final spoken = <String>[];
    final v = VoiceGuide(speak: (t) async => spoken.add(t));
    await v.say('2호선을 타고 5정거장 뒤 사당에서 내리세요', cueKey: 'L1:board');
    await v.say('2호선을 타고 4정거장 뒤 사당에서 내리세요', cueKey: 'L1:board'); // 역을 하나 지남
    await v.say('다음 역에서 내리세요', cueKey: 'L1:alight');
    expect(spoken, ['2호선을 타고 5정거장 뒤 사당에서 내리세요', '다음 역에서 내리세요']);
  });

  test('구간을 손으로 옮기면 그 구간 안내를 다시 읽는다', () async {
    final spoken = <String>[];
    final v = VoiceGuide(speak: (t) async => spoken.add(t));
    await v.say('탑승', cueKey: 'L1:board');
    await v.say('다른 구간', cueKey: 'L2:walk');
    v.forget('L1:');
    await v.say('탑승', cueKey: 'L1:board');
    await v.say('다른 구간', cueKey: 'L2:walk'); // 지우지 않은 구간은 그대로
    expect(spoken, ['탑승', '다른 구간', '탑승']);
  });

  test('꺼져 있거나 읽을 문장이 없으면 읽지 않고, 그 시점을 쓴 것으로 치지도 않는다', () async {
    final spoken = <String>[];
    final v = VoiceGuide(speak: (t) async => spoken.add(t), enabled: false);
    await v.say('탑승', cueKey: 'L1:board');
    v.enabled = true;
    await v.say(null, cueKey: 'L1:board');
    await v.say('탑승', cueKey: 'L1:board');
    expect(spoken, ['탑승']);
  });
}
