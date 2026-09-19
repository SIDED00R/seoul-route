import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/replace_guide_confirm.dart';

void main() {
  Future<List<bool>> host(WidgetTester tester) async {
    final answers = <bool>[];
    await tester.pumpWidget(MaterialApp(
      home: Builder(
        builder: (context) => TextButton(
          onPressed: () async => answers.add(await confirmReplaceGuide(context)),
          child: const Text('안내 시작'),
        ),
      ),
    ));
    return answers;
  }

  testWidgets('새로 시작을 누르면 true', (tester) async {
    final answers = await host(tester);
    await tester.tap(find.text('안내 시작'));
    await tester.pumpAndSettle();
    expect(find.text('하던 안내를 끝내고 새로 시작할까요?'), findsOneWidget);
    expect(find.textContaining('표본이 충분한'), findsOneWidget); // 서버는 이동 쌍 12개 미만이면 반영하지 않는다
    await tester.tap(find.text('새로 시작'));
    await tester.pumpAndSettle();
    expect(answers, [true]);
    expect(find.text('하던 안내를 끝내고 새로 시작할까요?'), findsNothing);
  });

  testWidgets('계속 안내나 바깥 탭은 false', (tester) async {
    final answers = await host(tester);
    await tester.tap(find.text('안내 시작'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('계속 안내'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('안내 시작'));
    await tester.pumpAndSettle();
    await tester.tapAt(const Offset(5, 5)); // 대화상자 바깥
    await tester.pumpAndSettle();
    expect(answers, [false, false]);
  });
}
