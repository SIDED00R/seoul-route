import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/status_notification.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const channel = MethodChannel('seoul_route/guide_status');
  final calls = <MethodCall>[];

  setUp(() {
    calls.clear();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call);
      return null;
    });
  });

  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(channel, null);
  });

  test('내용이 바뀔 때만 알림을 갱신한다', () async {
    final n = GuideStatusNotification();
    await n.show('3정거장 뒤 사당에서 내리기', '남은 20분');
    await n.show('3정거장 뒤 사당에서 내리기', '남은 20분'); // 2초마다 같은 내용이 온다
    await n.show('2정거장 뒤 사당에서 내리기', '남은 18분');
    expect(calls.map((c) => c.method), ['show', 'show']);
    expect(calls.last.arguments, {'title': '2정거장 뒤 사당에서 내리기', 'text': '남은 18분'});
    await n.cancel();
    expect(calls.last.method, 'cancel');
    await n.show('2정거장 뒤 사당에서 내리기', '남은 18분'); // 지운 뒤에는 같은 내용도 다시 띄운다
    expect(calls.last.method, 'show');
  });

  test('채널이 없어도 예외 없이 지나간다', () async {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(channel, null);
    final n = GuideStatusNotification();
    await n.show('a', 'b');
    await n.cancel();
  });
}
