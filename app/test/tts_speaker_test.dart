import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/tts_speaker.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  /// 폰 음성 엔진(flutter_tts)과 오디오 포커스(seoul_route/audio_focus) 채널을 흉내 내고 speak·stop·duck·release 를
  /// 차례로 적는다. 오디오 포커스 응답은 플랫폼 왕복처럼 늦게 온다.
  List<String> mockChannels() {
    const tts = MethodChannel('flutter_tts');
    const focus = MethodChannel('seoul_route/audio_focus');
    final calls = <String>[];
    final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(tts, (call) async {
      final args = call.arguments;
      if (call.method == 'speak') calls.add('speak:${args is Map ? args['text'] : args}');
      if (call.method == 'stop') calls.add('stop');
      return switch (call.method) {
        'getEngines' => ['com.google.android.tts'],
        'isLanguageAvailable' => true,
        _ => 1,
      };
    });
    messenger.setMockMethodCallHandler(focus, (call) async {
      calls.add(call.method);
      await Future<void>.delayed(const Duration(milliseconds: 5));
      return null;
    });
    addTearDown(() {
      messenger.setMockMethodCallHandler(tts, null);
      messenger.setMockMethodCallHandler(focus, null);
    });
    return calls;
  }

  test('잇달아 읽은 문장은 부른 순서대로 큐에 들어간다', () async {
    final calls = mockChannels();
    final speaker = (await TtsSpeaker.create())!;
    speaker.speak('A');
    speaker.speak('B');
    await Future<void>.delayed(const Duration(milliseconds: 20));
    expect(calls, ['duck', 'speak:A', 'speak:B']);
  });

  test('읽자마자 중지하면 그 문장이 중지 뒤에 큐에 들어가지 않는다', () async {
    final calls = mockChannels();
    final speaker = (await TtsSpeaker.create())!;
    speaker.speak('A');
    await speaker.stop();
    await Future<void>.delayed(const Duration(milliseconds: 20));
    expect(calls, ['duck', 'speak:A', 'stop', 'release']);
  });
}
