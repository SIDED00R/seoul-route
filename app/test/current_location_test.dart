import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:geolocator/geolocator.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/location/current_location.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/screens/place_search_screen.dart';
import 'package:seoul_route/screens/plan_screen.dart';
import 'package:seoul_route/settings/settings_store.dart';

const _settings = Settings(baseUrl: 'http://127.0.0.1:8081', token: 't');
const _a = Place(name: 'A역', address: 'a', lat: 37.55, lon: 126.97);
const _b = Place(name: 'B역', address: 'b', lat: 37.50, lon: 127.03);
const _here = Place(name: '현재 위치', address: '정확도 약 12m', lat: 37.5006, lon: 127.0364);

// 진행 표시가 도는 동안은 pumpAndSettle 이 끝나지 않으므로 프레임을 몇 번 넘긴다.
Future<void> _frames(WidgetTester t) async {
  for (var i = 0; i < 10; i++) {
    await t.pump(const Duration(milliseconds: 100));
  }
}

/// 장소 타일을 눌러 검색 화면을 열고 그 화면이 결과를 고른 것처럼 p 를 돌려준다.
Future<void> _pick(WidgetTester t, Finder tile, Place p) async {
  await t.tap(tile);
  await _frames(t);
  Navigator.pop(t.element(find.byType(PlaceSearchScreen)), p);
  await _frames(t);
}

bool _planEnabled(WidgetTester t) => t.widget<FilledButton>(find.byType(FilledButton)).onPressed != null;

/// 위치 권한·서비스는 켜진 것으로 보고 고정 좌표를 돌려준다.
class FakeGeolocator extends GeolocatorPlatform {
  @override
  Future<bool> isLocationServiceEnabled() async => true;

  @override
  Future<LocationPermission> checkPermission() async => LocationPermission.whileInUse;

  @override
  Future<Position> getCurrentPosition({LocationSettings? locationSettings}) async => Position(
        latitude: 37.5547,
        longitude: 126.9707,
        timestamp: DateTime.now(),
        accuracy: 12,
        altitude: 0,
        altitudeAccuracy: 0,
        heading: 0,
        headingAccuracy: 0,
        speed: 0,
        speedAccuracy: 0,
      );
}

/// 좌표 → 이름 조회 결과를 정해 주는 서버. name 이 null 이면 조회가 실패한다.
class NamingApi extends ApiClient {
  NamingApi({this.name, this.address = ''}) : super(baseUrl: 'http://x', token: 't');

  final String? name;
  final String address;

  @override
  Future<({String address, String name})> reversePlace(double lat, double lon) async {
    final n = name;
    if (n == null) throw ApiException(503, '장소 검색 미설정');
    return (name: n, address: address);
  }
}

void main() {
  test('현재 위치에 건물 이름과 도로명 주소를 붙인다', () async {
    GeolocatorPlatform.instance = FakeGeolocator();
    final p = await currentPlace(NamingApi(name: '서울역', address: '서울 중구 세종대로 2'));
    // 이름은 반드시 "현재 위치" 로 시작한다 — 서버는 첫 낱말이 "…역" 인 이름만 근처 역으로 앵커링한다.
    expect(p.name, '현재 위치 · 서울역');
    expect(p.address, '서울 중구 세종대로 2 · 정확도 약 12m');
    expect(p.lat, 37.5547);
  });

  test('이름을 못 받으면 좌표만 쓴다', () async {
    GeolocatorPlatform.instance = FakeGeolocator();
    expect((await currentPlace(NamingApi(name: null))).name, '현재 위치'); // 서버 미설정·오류
    final empty = await currentPlace(NamingApi(name: ''));
    expect(empty.name, '현재 위치'); // 건물도 주소도 없는 좌표
    expect(empty.address, '정확도 약 12m');
  });

  testWidgets('현재 위치 버튼: 받는 동안 진행 표시, 받으면 출발지가 현재 위치가 된다', (tester) async {
    final done = Completer<Place>();
    var calls = 0;
    await tester.pumpWidget(MaterialApp(
      home: PlanScreen(
        settings: _settings,
        locate: (_) {
          calls++;
          return done.future;
        },
      ),
    ));
    expect(find.text('출발지 선택'), findsOneWidget);
    await tester.tap(find.byTooltip('현재 위치'));
    await tester.pump();
    expect(calls, 1);
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    expect(find.byTooltip('현재 위치'), findsNothing); // 받는 동안 다시 누를 수 없다

    done.complete(const Place(name: '현재 위치', address: '정확도 약 12m', lat: 37.5006, lon: 127.0364));
    await tester.pumpAndSettle();
    expect(find.text('출발: 현재 위치'), findsOneWidget);
    expect(find.text('정확도 약 12m'), findsOneWidget);
    expect(find.byType(CircularProgressIndicator), findsNothing);
    expect(find.byTooltip('현재 위치'), findsOneWidget);
  });

  testWidgets('현재 위치 실패: 이유를 보여 주고 출발지는 그대로 둔다', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: PlanScreen(
        settings: _settings,
        locate: (_) async => throw const LocationException('위치 권한이 없습니다. 설정에서 허용한 뒤 다시 누르세요.'),
      ),
    ));
    await tester.tap(find.byTooltip('현재 위치'));
    await tester.pumpAndSettle();
    expect(find.text('위치 권한이 없습니다. 설정에서 허용한 뒤 다시 누르세요.'), findsOneWidget);
    expect(find.text('출발지 선택'), findsOneWidget);
    expect(find.byTooltip('현재 위치'), findsOneWidget);
  });

  testWidgets('위치를 받는 동안 경로 찾기는 꺼지고, 끝나면(성공·실패) 다시 켜진다', (tester) async {
    var loc = Completer<Place>();
    await tester.pumpWidget(MaterialApp(home: PlanScreen(settings: _settings, locate: (_) => loc.future)));
    await _pick(tester, find.text('출발지 선택'), _a);
    await _pick(tester, find.text('도착지 선택'), _b);
    expect(_planEnabled(tester), isTrue);

    await tester.tap(find.byTooltip('현재 위치'));
    await tester.pump();
    expect(_planEnabled(tester), isFalse); // 이대로 누르면 A역 기준으로 요청이 나간다
    loc.complete(_here);
    await _frames(tester);
    expect(find.text('출발: 현재 위치'), findsOneWidget);
    expect(_planEnabled(tester), isTrue);

    loc = Completer<Place>();
    await tester.tap(find.byTooltip('현재 위치'));
    await tester.pump();
    expect(_planEnabled(tester), isFalse);
    loc.completeError(const LocationException('15초 안에 위치를 받지 못했습니다. 잠시 뒤 다시 누르세요.'));
    await _frames(tester);
    expect(find.text('출발: 현재 위치'), findsOneWidget);
    expect(_planEnabled(tester), isTrue);
  });

  testWidgets('위치를 받는 동안 출발지 검색은 열리지 않아 늦게 온 위치가 수동 선택을 덮지 않는다', (tester) async {
    final loc = Completer<Place>();
    await tester.pumpWidget(MaterialApp(home: PlanScreen(settings: _settings, locate: (_) => loc.future)));
    await tester.tap(find.byTooltip('현재 위치'));
    await tester.pump();
    await tester.tap(find.text('출발지 선택'));
    await _frames(tester);
    expect(find.byType(PlaceSearchScreen), findsNothing);

    loc.complete(_here);
    await _frames(tester);
    expect(find.text('출발: 현재 위치'), findsOneWidget);
    await _pick(tester, find.text('출발: 현재 위치'), _a); // 끝난 뒤에는 검색으로 바꿀 수 있다
    expect(find.text('출발: A역'), findsOneWidget);
  });

  testWidgets('경로 요청이 나가는 동안 현재 위치 버튼·출발지 검색이 막히고, 끝나면 다시 된다', (tester) async {
    final reply = Completer<http.Response>();
    final mock = MockClient((_) => reply.future);
    await http.runWithClient(() async {
      await tester.pumpWidget(MaterialApp(home: PlanScreen(settings: _settings, locate: (_) async => _here)));
      await _pick(tester, find.text('출발지 선택'), _a);
      await _pick(tester, find.text('도착지 선택'), _b);
      IconButton here() => tester.widget<IconButton>(find.widgetWithIcon(IconButton, Icons.my_location));
      expect(here().onPressed, isNotNull);

      await tester.tap(find.byType(FilledButton));
      await tester.pump();
      expect(here().onPressed, isNull); // 이대로 누르면 홈 출발지가 요청과 달라진다
      // 꺼진 아이콘을 눌러도 탭이 감싼 출발지 줄로 넘어가 검색이 열리지 않아야 한다. 줄 본문도 마찬가지.
      await tester.tap(find.byIcon(Icons.my_location));
      await _frames(tester);
      expect(find.byType(PlaceSearchScreen), findsNothing);
      await tester.tap(find.text('출발: A역'));
      await _frames(tester);
      expect(find.byType(PlaceSearchScreen), findsNothing);

      reply.complete(http.Response('{"error":"x"}', 500, headers: {'content-type': 'application/json'}));
      await _frames(tester);
      expect(here().onPressed, isNotNull);
      expect(find.text('출발: A역'), findsOneWidget);
      await _pick(tester, find.text('출발: A역'), _b); // 요청이 끝나면 검색으로 바꿀 수 있다
      expect(find.text('출발: B역'), findsOneWidget);
    }, () => mock);
  });
}
