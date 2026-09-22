import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/screens/favorite_places_screen.dart';
import 'package:seoul_route/screens/plan_screen.dart';
import 'package:seoul_route/settings/settings_store.dart';

void main() {
  testWidgets('자주 가는 곳 칩을 누르면 도착지가 바로 채워진다', (tester) async {
    await http.runWithClient(
      () async {
        await tester.pumpWidget(
          const MaterialApp(
            home: PlanScreen(
              settings: Settings(baseUrl: 'http://server', token: 'token'),
            ),
          ),
        );
        await tester.pumpAndSettle();
        expect(find.text('자주 가는 곳'), findsOneWidget);
        expect(find.text('집'), findsOneWidget);
        await tester.tap(find.widgetWithText(ActionChip, '집'));
        await tester.pump();
        expect(find.text('도착: 우리집'), findsOneWidget);
      },
      () => MockClient((request) async {
        expect(request.url.path, '/users/me/favorites');
        return http.Response(
          '{"favorites":[{"id":"1","kind":"home","label":"집",'
          '"place":{"name":"우리집","address":"서울 양천구","lat":37.53,"lon":126.87}}]}',
          200,
          headers: const {'content-type': 'application/json; charset=utf-8'},
        );
      }),
    );
  });

  testWidgets('즐겨찾기 편집 창을 뒤로 닫아도 화면 정리 오류가 나지 않는다', (tester) async {
    await http.runWithClient(
      () async {
        await tester.pumpWidget(
          MaterialApp(
            home: FavoritePlacesScreen(
              api: ApiClient(baseUrl: 'http://server', token: 'token'),
            ),
          ),
        );
        await tester.pumpAndSettle();
        await tester.tap(find.byType(ListTile));
        await tester.pumpAndSettle();
        expect(find.text('즐겨찾기 수정'), findsOneWidget);

        await tester.binding.handlePopRoute();
        await tester.pumpAndSettle();

        expect(find.text('즐겨찾기 수정'), findsNothing);
        expect(tester.takeException(), isNull);
      },
      () => MockClient(
        (_) async => http.Response(
          '{"favorites":[{"id":"station","kind":"custom","label":"서울역",'
          '"place":{"name":"서울역","address":"서울 중구 한강대로 405",'
          '"lat":37.5547,"lon":126.9707}}]}',
          200,
          headers: const {'content-type': 'application/json; charset=utf-8'},
        ),
      ),
    );
  });

  testWidgets('계정이 바뀌면 이전 즐겨찾기를 지우고 늦은 응답을 무시한다', (tester) async {
    final second = Completer<http.Response>();
    final third = Completer<http.Response>();
    await http.runWithClient(
      () async {
        Widget screen(String token) => MaterialApp(
          home: PlanScreen(
            settings: Settings(baseUrl: 'http://server', token: token),
          ),
        );

        await tester.pumpWidget(screen('first'));
        await tester.pumpAndSettle();
        expect(find.text('첫 계정'), findsOneWidget);

        await tester.pumpWidget(screen('second'));
        await tester.pump();
        expect(
          find.text('첫 계정'),
          findsNothing,
          reason: '계정 변경 즉시 이전 위치를 숨겨야 한다',
        );

        await tester.pumpWidget(screen('third'));
        await tester.pump();
        third.complete(_favoritesResponse('세 번째 계정'));
        await tester.pumpAndSettle();
        expect(find.text('세 번째 계정'), findsOneWidget);

        second.complete(_favoritesResponse('두 번째 계정'));
        await tester.pumpAndSettle();
        expect(find.text('세 번째 계정'), findsOneWidget);
        expect(
          find.text('두 번째 계정'),
          findsNothing,
          reason: '이전 계정의 늦은 응답을 무시해야 한다',
        );
      },
      () => MockClient((request) {
        switch (request.headers['authorization']) {
          case 'Bearer first':
            return Future.value(_favoritesResponse('첫 계정'));
          case 'Bearer second':
            return second.future;
          case 'Bearer third':
            return third.future;
          default:
            throw StateError('unexpected authorization');
        }
      }),
    );
  });

  testWidgets('삭제 요청 중에는 편집과 중복 삭제를 막는다', (tester) async {
    final deleted = Completer<http.Response>();
    var deleteCalls = 0;
    var exists = true;
    await http.runWithClient(
      () async {
        await tester.pumpWidget(
          MaterialApp(
            home: FavoritePlacesScreen(
              api: ApiClient(baseUrl: 'http://server', token: 'token'),
            ),
          ),
        );
        await tester.pumpAndSettle();
        await tester.tap(find.byTooltip('삭제'));
        await tester.pumpAndSettle();
        await tester.tap(find.widgetWithText(FilledButton, '삭제'));
        await tester.pump();

        final deleteButton = find.ancestor(
          of: find.byIcon(Icons.delete_outline),
          matching: find.byType(IconButton),
        );
        expect(tester.widget<IconButton>(deleteButton).onPressed, isNull);
        expect(tester.widget<ListTile>(find.byType(ListTile)).onTap, isNull);
        expect(deleteCalls, 1);

        exists = false;
        deleted.complete(http.Response('', 204));
        await tester.pumpAndSettle();
        expect(find.byType(ListTile), findsNothing);
      },
      () => MockClient((request) async {
        if (request.method == 'DELETE') {
          deleteCalls++;
          return deleted.future;
        }
        return exists ? _favoritesResponse('삭제 대상') : _favoritesResponse(null);
      }),
    );
  });
}

http.Response _favoritesResponse(String? label) => http.Response(
  label == null
      ? '{"favorites":[]}'
      : '{"favorites":[{"id":"1","kind":"custom","label":"$label",'
            '"place":{"name":"장소","address":"서울","lat":37.53,"lon":126.87}}]}',
  200,
  headers: const {'content-type': 'application/json; charset=utf-8'},
);
