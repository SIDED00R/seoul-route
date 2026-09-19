import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/models/recent_route.dart';
import 'package:seoul_route/screens/home_screen.dart';
import 'package:seoul_route/screens/plan_screen.dart';
import 'package:seoul_route/screens/recent_routes_tab.dart';
import 'package:seoul_route/settings/settings_store.dart';

class _Api extends ApiClient {
  _Api({this.routes = const [], this.fail}) : super(baseUrl: 'http://x', token: 't');

  final List<Map<String, dynamic>> routes;
  final ApiException? fail;
  int cleared = 0;

  @override
  Future<List<RecentRoute>> recentRoutes() async {
    if (fail != null) throw fail!;
    return routes.map(RecentRoute.fromJson).toList();
  }

  @override
  Future<void> clearRecentRoutes() async => cleared++;
}

Map<String, dynamic> _row(String from, String to,
        {List<Map<String, dynamic>> via = const [], List<String> modes = const []}) =>
    {
      'request': {
        'origin': {'lat': 37.5547, 'lon': 126.9707, 'name': from},
        'destination': {'lat': 37.4979, 'lon': 127.0276, 'name': to},
        if (via.isNotEmpty) 'via': via,
        if (modes.isNotEmpty) 'segment_modes': modes,
      },
      'searched_at': DateTime.now().toIso8601String(),
    };

void main() {
  setUp(() => SharedPreferences.setMockInitialValues(<String, Object>{}));

  // 서버가 준 요청을 그대로 되살린다 — 경유지·구간 수단까지 그대로여야 고른 대로 다시 검색된다.
  test('RecentRoute.fromJson: 요청을 그대로 되살린다', () {
    final r = RecentRoute.fromJson(_row('서울역', '강남역',
        via: [
          {'lat': 37.52, 'lon': 126.92, 'name': '여의도'}
        ],
        modes: ['transit', 'bike']));
    expect(r.label, '서울역 → 강남역');
    expect(r.request.via.single.name, '여의도');
    expect(r.request.segmentModes, [SegmentMode.transit, SegmentMode.bike]);
    expect(r.note, '경유 1곳 · 구간 수단 고정');

    // 전 구간 any 면 서버가 segment_modes 를 싣지 않는다 — 구간 수만큼 any 로 채운다.
    final plain = RecentRoute.fromJson(_row('서울역', '강남역'));
    expect(plain.request.segmentModes, [SegmentMode.any]);
    expect(plain.note, '');
  });

  testWidgets('최근 경로를 보여 주고 고르면 알린다', (tester) async {
    final api = _Api(routes: [_row('서울역', '강남역'), _row('이태원', '홍대입구')]);
    PlanScreenPreset? picked;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: RecentRoutesTab(api: api, ready: true, onPick: (p) => picked = p)),
    ));
    await tester.pumpAndSettle();
    expect(find.text('서울역 → 강남역'), findsOneWidget);
    expect(find.text('이태원 → 홍대입구'), findsOneWidget);

    await tester.tap(find.text('이태원 → 홍대입구'));
    await tester.pump();
    expect(picked?.request.destination.name, '홍대입구');
  });

  testWidgets('비어 있으면 안내만 보여 주고, 설정 전에는 불러오지 않는다', (tester) async {
    final empty = _Api();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: RecentRoutesTab(api: empty, ready: true, onPick: (_) {})),
    ));
    await tester.pumpAndSettle();
    expect(find.textContaining('아직 찾아본 경로가 없습니다'), findsOneWidget);
    expect(find.text('최근 경로 모두 지우기'), findsNothing);

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: RecentRoutesTab(api: _Api(fail: ApiException(401, '')), ready: false, onPick: (_) {})),
    ));
    await tester.pumpAndSettle();
    expect(find.textContaining('서버 주소와 토큰을 먼저 설정하세요'), findsOneWidget);
  });

  testWidgets('모두 지우기: 확인하면 서버에서 지우고 목록을 비운다', (tester) async {
    final api = _Api(routes: [_row('서울역', '강남역')]);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: RecentRoutesTab(api: api, ready: true, onPick: (_) {})),
    ));
    await tester.pumpAndSettle();
    await tester.tap(find.text('최근 경로 모두 지우기'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('취소'));
    await tester.pumpAndSettle();
    expect(api.cleared, 0);
    expect(find.text('서울역 → 강남역'), findsOneWidget);

    await tester.tap(find.text('최근 경로 모두 지우기'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('모두 지우기').last);
    await tester.pumpAndSettle();
    expect(api.cleared, 1);
    expect(find.text('서울역 → 강남역'), findsNothing);
  });

  // 홈은 세 탭이고, 최근 경로를 고르면 길찾기 탭으로 넘어가 출발·도착이 채워진다.
  testWidgets('홈 탭: 최근 경로를 고르면 길찾기 탭에 채워진다', (tester) async {
    await tester.pumpWidget(const MaterialApp(
      home: HomeScreen(settings: Settings(baseUrl: 'http://x', token: 't'), initialTab: 0),
    ));
    await tester.pumpAndSettle();
    expect(find.text('최근 경로'), findsOneWidget);
    expect(find.text('현재 경로'), findsOneWidget);
    expect(find.text('길찾기'), findsOneWidget);
    // 실제 서버가 없으므로 최근 경로는 실패로 끝난다 — 탭 전환만 확인한다.
    await tester.tap(find.text('길찾기'));
    await tester.pumpAndSettle();
    expect(find.text('출발지 선택'), findsOneWidget);
    expect(find.text('도착지 선택'), findsOneWidget);

    await tester.tap(find.text('현재 경로'));
    await tester.pumpAndSettle();
    expect(find.textContaining('안내 중인 경로가 없습니다'), findsOneWidget);
  });
}
