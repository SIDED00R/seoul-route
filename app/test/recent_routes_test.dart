import 'dart:async';

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
  _Api({this.routes = const [], this.fail, this.delay = Duration.zero, this.holdFrom, String tok = 't'})
      : super(baseUrl: 'http://x', token: tok);

  final List<Map<String, dynamic>> routes;
  final ApiException? fail;
  final Duration delay;

  /// 이 번째 조회부터는 완료될 때까지 붙잡는다(응답 순서를 테스트가 정하려고). 1 이면 첫 조회부터.
  final int? holdFrom;
  final held = Completer<void>();

  /// 붙잡힌 조회가 돌려줄 목록. 앞선 응답과 달라야 화면에 반영됐는지 구분된다.
  List<Map<String, dynamic>>? heldRoutes;

  /// 삭제 응답을 붙잡는다. 붙잡는 동안 새 조회를 띄워 순서를 만든다.
  Completer<void>? holdClear;
  int cleared = 0;
  int loads = 0;

  @override
  Future<List<RecentRoute>> recentRoutes() async {
    loads++;
    final holding = holdFrom != null && loads >= holdFrom!;
    if (holding) await held.future;
    if (delay > Duration.zero) await Future<void>.delayed(delay);
    if (fail != null) throw fail!;
    final rows = holding ? (heldRoutes ?? routes) : routes;
    return rows.map(RecentRoute.fromJson).toList();
  }

  @override
  Future<void> clearRecentRoutes() async {
    cleared++;
    if (holdClear != null) await holdClear!.future;
  }
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

  // 삭제를 기다리는 동안 먼저 떠 있던 조회가 돌아온다. 그 응답을 그리면 지운 기록이 화면에 되살아난다.
  testWidgets('모두 지우기: 삭제 전에 떠 있던 조회가 지운 목록을 되살리지 않는다', (tester) async {
    final api = _Api(routes: [_row('서울역', '강남역')], holdFrom: 2)
      ..holdClear = Completer<void>()
      ..heldRoutes = [_row('이태원', '홍대입구')];
    Widget tab(int key) =>
        MaterialApp(home: Scaffold(body: RecentRoutesTab(api: api, ready: true, refreshKey: key, onPick: (_) {})));
    await tester.pumpWidget(tab(0));
    await tester.pumpAndSettle();
    expect(find.text('서울역 → 강남역'), findsOneWidget);

    await tester.pumpWidget(tab(1)); // 탭 재진입 → 두 번째 조회가 뜨고 붙잡힌다
    await tester.pump();
    expect(api.loads, 2);

    await tester.tap(find.text('최근 경로 모두 지우기'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('모두 지우기').last);
    await tester.pump();
    expect(api.cleared, 1);

    api.held.complete(); // 삭제가 아직 진행 중인 사이에 그 조회가 돌아온다
    await tester.pumpAndSettle();
    expect(find.text('이태원 → 홍대입구'), findsNothing);

    api.holdClear!.complete();
    await tester.pumpAndSettle();
    expect(find.text('서울역 → 강남역'), findsNothing);
    expect(find.text('이태원 → 홍대입구'), findsNothing);
  });

  // 삭제를 기다리는 동안 탭을 다시 들어오면 새 조회가 뜬다. 그 조회는 삭제 전 목록을 읽었을 수 있다.
  testWidgets('모두 지우기: 삭제 중에 뜬 조회도 지운 목록을 되살리지 않는다', (tester) async {
    final api = _Api(routes: [_row('서울역', '강남역')], holdFrom: 2)..holdClear = Completer<void>();
    Widget tab(int key) =>
        MaterialApp(home: Scaffold(body: RecentRoutesTab(api: api, ready: true, refreshKey: key, onPick: (_) {})));
    await tester.pumpWidget(tab(0));
    await tester.pumpAndSettle();

    await tester.tap(find.text('최근 경로 모두 지우기'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('모두 지우기').last);
    await tester.pump(); // DELETE 가 붙잡혀 있다
    expect(api.cleared, 1);

    await tester.pumpWidget(tab(1)); // 삭제 대기 중 탭 재진입 → 새 조회
    await tester.pump();
    expect(api.loads, 2);

    api.holdClear!.complete();
    await tester.pumpAndSettle();
    expect(find.text('서울역 → 강남역'), findsNothing);

    api.held.complete(); // 삭제 중에 떴던 조회가 이제 돌아온다
    await tester.pumpAndSettle();
    expect(find.text('서울역 → 강남역'), findsNothing);
  });

  // 계정이 바뀌면 남아 있는 목록은 남의 기록이다. 먼저 비우고 새로 받아야 한다.
  testWidgets('계정이 바뀌면 이전 기록을 비우고 다시 받는다', (tester) async {
    final mine = _Api(routes: [_row('서울역', '강남역')]);
    final other = _Api(routes: [_row('이태원', '홍대입구')], tok: 'other');
    Widget tab(_Api api) => MaterialApp(home: Scaffold(body: RecentRoutesTab(api: api, ready: true, onPick: (_) {})));
    await tester.pumpWidget(tab(mine));
    await tester.pumpAndSettle();
    expect(find.text('서울역 → 강남역'), findsOneWidget);

    await tester.pumpWidget(tab(other));
    await tester.pump(); // 받아 오기 전에 이미 비어 있어야 한다
    expect(find.text('서울역 → 강남역'), findsNothing);
    await tester.pumpAndSettle();
    expect(find.text('이태원 → 홍대입구'), findsOneWidget);
    expect(other.loads, 1);
  });

  // 조회 중에 계정이 바뀌면, 늦게 돌아온 이전 계정 응답이 화면을 덮으면 안 되고 새 계정은 반드시 받아야 한다.
  testWidgets('조회 중 계정이 바뀌어도 이전 계정 응답이 남지 않는다', (tester) async {
    final mine = _Api(routes: [_row('서울역', '강남역')], delay: const Duration(milliseconds: 300));
    final other = _Api(routes: [_row('이태원', '홍대입구')], tok: 'other');
    Widget tab(_Api api) => MaterialApp(home: Scaffold(body: RecentRoutesTab(api: api, ready: true, onPick: (_) {})));
    await tester.pumpWidget(tab(mine));
    await tester.pump(); // 조회를 걸어 둔 채로

    await tester.pumpWidget(tab(other)); // 계정 전환
    await tester.pumpAndSettle(const Duration(milliseconds: 500));
    expect(other.loads, 1); // 새 계정을 받았다
    expect(find.text('이태원 → 홍대입구'), findsOneWidget);
    expect(find.text('서울역 → 강남역'), findsNothing); // 늦게 온 이전 계정 응답이 덮지 않았다
  });

  // 길찾기 탭에서 방금 찾은 경로가 보이려면 탭에 들어올 때마다 다시 받아야 한다.
  testWidgets('탭에 들어올 때마다 목록을 다시 받는다', (tester) async {
    final api = _Api(routes: [_row('서울역', '강남역')]);
    Widget tab(int key) => MaterialApp(
        home: Scaffold(body: RecentRoutesTab(api: api, ready: true, onPick: (_) {}, refreshKey: key)));
    await tester.pumpWidget(tab(0));
    await tester.pumpAndSettle();
    expect(api.loads, 1);

    await tester.pumpWidget(tab(0));
    await tester.pumpAndSettle();
    expect(api.loads, 1); // 탭을 떠나 있는 동안에는 다시 받지 않는다

    await tester.pumpWidget(tab(1));
    await tester.pumpAndSettle();
    expect(api.loads, 2);
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
