import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/screens/place_search_screen.dart';

/// 첫 질의('강')가 느리고 두 번째가 빠른 상황 = 모바일망에서 흔한 응답 순서 역전.
class SlowFirstApi extends ApiClient {
  SlowFirstApi() : super(baseUrl: 'http://x', token: 't');

  @override
  Future<List<Place>> searchPlaces(String q, {({double lat, double lon})? near}) async {
    await Future.delayed(Duration(milliseconds: q == '강' ? 1500 : 100));
    return [Place(name: '결과:$q', address: '주소', lat: 0, lon: 0)];
  }
}

/// 받은 위치를 기록하고, 위치가 있으면 거리가 붙은 결과를 준다.
class NearApi extends ApiClient {
  NearApi() : super(baseUrl: 'http://x', token: 't');

  final nears = <({double lat, double lon})?>[];

  @override
  Future<List<Place>> searchPlaces(String q, {({double lat, double lon})? near}) async {
    nears.add(near);
    return [
      Place(name: '가나커피 앞점', address: '가나로 1', category: '카페', lat: 0, lon: 0, distanceM: near == null ? null : 1234),
    ];
  }
}

void main() {
  test('검색 요청에 위치를 lat·lon 으로 싣고 거리를 읽는다. 위치가 없으면 q 만 보낸다', () async {
    final got = <Uri>[];
    final mock = MockClient((req) async {
      got.add(req.url);
      return http.Response('{"places":[{"name":"가나","address":"a","lat":37.5,"lon":127.0,"distance_m":850}]}', 200,
          headers: {'content-type': 'application/json; charset=utf-8'});
    });
    final api = ApiClient(baseUrl: 'http://x', token: 't');
    final places = await http.runWithClient(() => api.searchPlaces('가나', near: (lat: 37.5, lon: 127.0)), () => mock);
    await http.runWithClient(() => api.searchPlaces('가나'), () => mock);
    expect(got[0].queryParameters, {'q': '가나', 'lat': '37.5', 'lon': '127.0'});
    expect(got[1].queryParameters, {'q': '가나'});
    expect(places.single.distanceM, 850);
  });

  testWidgets('화면을 열 때 받은 위치를 검색에 실어 보내고 거리를 보여 준다', (tester) async {
    final api = NearApi();
    await tester.pumpWidget(MaterialApp(
      home: PlaceSearchScreen(api: api, title: 't', locate: () async => (lat: 37.5, lon: 127.0)),
    ));
    await tester.enterText(find.byType(TextField), '가나커피');
    await tester.pump(const Duration(milliseconds: 500));
    await tester.pump();
    expect(api.nears, [(lat: 37.5, lon: 127.0)]);
    expect(find.text('1.2km · 카페 · 가나로 1'), findsOneWidget);
  });

  testWidgets('위치를 못 받으면 위치 없이 검색하고 거리를 쓰지 않는다', (tester) async {
    final api = NearApi();
    await tester.pumpWidget(MaterialApp(home: PlaceSearchScreen(api: api, title: 't', locate: () async => null)));
    await tester.enterText(find.byType(TextField), '가나커피');
    await tester.pump(const Duration(milliseconds: 500));
    await tester.pump();
    expect(api.nears, [null]);
    expect(find.text('카페 · 가나로 1'), findsOneWidget);
  });

  testWidgets('늦게 온 이전 응답은 최신 결과를 덮지 않는다', (tester) async {
    await tester.pumpWidget(
        MaterialApp(home: PlaceSearchScreen(api: SlowFirstApi(), title: 't', locate: () async => null)));
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
    await tester.pumpWidget(
        MaterialApp(home: PlaceSearchScreen(api: SlowFirstApi(), title: 't', locate: () async => null)));
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
