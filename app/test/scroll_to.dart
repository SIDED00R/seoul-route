import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

/// 설정 화면처럼 긴 ListView 의 아래쪽 위젯(저장·연결 확인·상태 문구)이 기본 800px 테스트 화면 밖에 있으면
/// 만들어지지도 않는다. 화면을 키워 목록 전체를 보이게 한다(드래그로 스크롤하면 뒤에 남은 홈 화면까지 밀려 깨진다).
Future<void> scrollTo(WidgetTester tester, Finder target) async {
  tester.view.physicalSize = const Size(1440, 4000);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpAndSettle();
  expect(target, findsOneWidget);
}
