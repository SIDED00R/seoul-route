import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';
import 'package:seoul_route/screens/results_screen.dart';
import 'package:seoul_route/widgets/mode_icon.dart';

/// 서버 응답 형식의 여정 하나. replanned 가 null 이면 키를 넣지 않는다(구버전 서버 응답).
Map<String, dynamic> _it({bool? replanned}) {
  final m = <String, dynamic>{
    'start': 's',
    'end': 'e',
    'duration_sec': 1500,
    'crossing_wait_sec': 114,
    'legs': [
      {'mode': 'WALK', 'duration_sec': 300, 'distance_m': 400, 'from_lat': 37.55, 'from_lon': 126.97,
       'to_lat': 37.551, 'to_lon': 126.971, 'transit_leg': false},
    ],
  };
  if (replanned != null) m['replanned'] = replanned;
  return m;
}

// 노선 색이 오면 칩을 그 색으로 칠하고, 없으면 기본 팔레트(흰 칩)를 쓴다.
Map<String, dynamic> _coloredLeg({String color = '', String textColor = ''}) => {
      'mode': 'SUBWAY',
      'duration_sec': 600.0,
      'distance_m': 5000.0,
      'from_lat': 37.5,
      'from_lon': 127.0,
      'to_lat': 37.51,
      'to_lon': 127.01,
      'route': '2호선',
      'transit_leg': true,
      if (color.isNotEmpty) 'color': color,
      if (textColor.isNotEmpty) 'text_color': textColor,
    };

void main() {
  test('색 문자열 해석: 6자리 16진수만 받는다', () {
    expect(parseHexColor('00A84D'), const Color(0xFF00A84D));
    expect(parseHexColor(''), isNull);
    expect(parseHexColor('#00A84D'), isNull); // GTFS 는 # 를 붙이지 않는다
    expect(parseHexColor('00A84'), isNull);
    expect(parseHexColor('GGGGGG'), isNull);
  });

  test('역 안 환승 통로 표시(in_station)를 읽는다', () {
    expect(Leg.fromJson({..._coloredLeg(), 'in_station': true}).inStation, isTrue);
    expect(Leg.fromJson(_coloredLeg()).inStation, isFalse);
  });

  test('leg 색이 있으면 그 색, 없으면 수단 기본색', () {
    final colored = Leg.fromJson(_coloredLeg(color: '00A84D', textColor: 'FFFFFF'));
    expect(modeColor(colored), const Color(0xFF00A84D));
    expect(legTextColor(colored), const Color(0xFFFFFFFF));
    final plain = Leg.fromJson(_coloredLeg());
    expect(modeColor(plain), defaultModeColor(plain));
    expect(legTextColor(plain), Colors.white); // 글자색이 없으면 흰색
  });

  testWidgets('결과 목록의 구간 칩은 노선 색으로 칠한다', (tester) async {
    final result = PlanResult.fromJson({
      'itineraries': [
        {
          'start': '2026-09-18T09:00:00+09:00',
          'end': '2026-09-18T09:30:00+09:00',
          'duration_sec': 1800.0,
          'transfers': 0,
          'walk_distance_m': 100.0,
          'legs': [_coloredLeg(color: '00A84D', textColor: 'FFFFFF')],
        }
      ]
    });
    await tester.pumpWidget(MaterialApp(
      home: ResultsScreen(
        api: ApiClient(baseUrl: 'http://127.0.0.1:8081', token: 't'),
        request: const PlanRequest(
          origin: Place(name: 'A', address: '', lat: 37.5, lon: 127.0),
          destination: Place(name: 'B', address: '', lat: 37.51, lon: 127.01),
        ),
        result: result,
      ),
    ));
    await tester.pump();
    final chip = tester.widget<Chip>(find.widgetWithText(Chip, '2호선 10분'));
    expect(chip.backgroundColor, const Color(0xFF00A84D));
    expect((chip.label as Text).style?.color, const Color(0xFFFFFFFF));
  });
  test('replanned 가 true 일 때만 재탐색 배지 문구', () {
    expect(Itinerary.fromJson(_it(replanned: true)).replannedLabel, '재탐색');
    expect(Itinerary.fromJson(_it(replanned: false)).replannedLabel, isNull);
    expect(Itinerary.fromJson(_it()).replannedLabel, isNull);
  });

  testWidgets('결과 목록은 재탐색 여정에 배지를 붙인다', (tester) async {
    final result = PlanResult.fromJson({
      'itineraries': [_it(replanned: true), _it(replanned: false)],
    });
    const p = Place(name: 'A역', address: '', lat: 37.55, lon: 126.97);
    await tester.pumpWidget(MaterialApp(
      home: ResultsScreen(
        api: ApiClient(baseUrl: 'http://127.0.0.1:8081', token: 't'),
        request: const PlanRequest(origin: p, destination: p),
        result: result,
      ),
    ));
    expect(find.text('재탐색'), findsOneWidget);
    expect(find.text('횡단보도 +2분'), findsNWidgets(2));
  });
}
