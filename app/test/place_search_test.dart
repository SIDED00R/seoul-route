import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/screens/place_search_screen.dart';

/// 첫 질의('강')가 느리고 두 번째가 빠른 상황 = 모바일망에서 흔한 응답 순서 역전.
class SlowFirstApi extends ApiClient {
  SlowFirstApi() : super(baseUrl: 'http://x', token: 't');

  @override
  Future<List<Place>> searchPlaces(String q) async {
    await Future.delayed(Duration(milliseconds: q == '강' ? 1500 : 100));
    return [Place(name: '결과:$q', address: '주소', lat: 0, lon: 0)];
  }
}

void main() {
  testWidgets('늦게 온 이전 응답은 최신 결과를 덮지 않는다', (tester) async {
    await tester.pumpWidget(MaterialApp(home: PlaceSearchScreen(api: SlowFirstApi(), title: 't')));
    await tester.enterText(find.byType(TextField), '강');
    await tester.pump(const Duration(milliseconds: 500)); // 디바운스 → '강' 요청(1500ms)
    await tester.enterText(find.byType(TextField), '강남');
    await tester.pump(const Duration(milliseconds: 500)); // 디바운스 → '강남' 요청(100ms)
    await tester.pump(const Duration(milliseconds: 200));
    expect(find.text('결과:강남'), findsOneWidget);
    expect(find.byType(CircularProgressIndicator), findsNothing);

    await tester.pump(const Duration(milliseconds: 1500)); // 뒤늦게 '강' 응답
    expect(find.text('결과:강남'), findsOneWidget);
    expect(find.text('결과:강'), findsNothing);
  });

  testWidgets('입력을 비운 뒤 도착한 응답은 목록을 다시 채우지 않고 스피너도 꺼진다', (tester) async {
    await tester.pumpWidget(MaterialApp(home: PlaceSearchScreen(api: SlowFirstApi(), title: 't')));
    await tester.enterText(find.byType(TextField), '강');
    await tester.pump(const Duration(milliseconds: 500));
    await tester.enterText(find.byType(TextField), '');
    await tester.pump(const Duration(milliseconds: 500));
    expect(find.text('결과:강'), findsNothing);
    expect(find.byType(CircularProgressIndicator), findsNothing);

    await tester.pump(const Duration(milliseconds: 1500));
    expect(find.text('결과:강'), findsNothing);
  });
}
