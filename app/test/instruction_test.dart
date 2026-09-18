import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/instruction.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/leg_detail.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';

const origin = Place(name: '집 앞', address: '', lat: 37.5, lon: 127.0);
const dest = Place(name: '회사', address: '', lat: 37.52, lon: 127.04);
const request = PlanRequest(origin: origin, destination: dest);

Leg mk(
  String mode, {
  String route = '',
  String headsign = '',
  bool transit = false,
  bool rented = false,
  String fromName = 'a',
  String toName = 'b',
  double durationSec = 300,
  List<WalkStep> steps = const [],
  List<TransitStop> stops = const [],
}) =>
    Leg(
      mode: mode,
      durationSec: durationSec,
      distanceM: 300,
      fromName: fromName,
      toName: toName,
      fromLat: 37.5,
      fromLon: 127.0,
      toLat: 37.502,
      toLon: 127.0,
      route: route,
      rentedBike: rented,
      transitLeg: transit,
      polyline: '',
      start: '',
      end: '',
      headsign: headsign,
      steps: steps,
      stops: stops,
    );

Itinerary it(List<Leg> legs) => Itinerary(
      start: '',
      end: '',
      durationSec: 1800,
      transfers: 0,
      walkM: 500,
      legs: legs,
    );

const walkSteps = [
  WalkStep(dir: 'DEPART', street: '테헤란로', distanceM: 124, lat: 37.5, lon: 127.0),
  WalkStep(dir: 'RIGHT', distanceM: 46, lat: 37.501, lon: 127.0),
  WalkStep(dir: 'ENTER_STATION', entrance: '강남 8번 출구', distanceM: 0, lat: 37.5015, lon: 127.0),
];

void main() {
  test('회전 문구표', () {
    expect(turnPhrase('DEPART'), '출발');
    expect(turnPhrase('CONTINUE'), '직진');
    expect(turnPhrase('SLIGHTLY_LEFT'), '왼쪽으로 약간');
    expect(turnPhrase('SLIGHTLY_RIGHT'), '오른쪽으로 약간');
    expect(turnPhrase('LEFT'), '좌회전');
    expect(turnPhrase('RIGHT'), '우회전');
    expect(turnPhrase('HARD_LEFT'), '왼쪽으로 크게');
    expect(turnPhrase('HARD_RIGHT'), '오른쪽으로 크게');
    expect(turnPhrase('UTURN_LEFT'), '유턴');
    expect(turnPhrase('UTURN_RIGHT'), '유턴');
    expect(turnPhrase('CIRCLE_CLOCKWISE'), '회전교차로 진입');
    expect(turnPhrase('CIRCLE_COUNTERCLOCKWISE', exit: '3'), '회전교차로 3번 출구');
    expect(turnPhrase('ELEVATOR'), '엘리베이터 이용');
    expect(turnPhrase('ENTER_STATION'), '역으로 들어가기');
    expect(turnPhrase('ENTER_STATION', entrance: '강남 8번 출구'), '강남 8번 출구로 들어가기');
    expect(turnPhrase('EXIT_STATION', entrance: '역삼 1번 출구'), '역삼 1번 출구로 나가기');
    expect(turnPhrase('FOLLOW_SIGNS'), '표지판 따라가기');
    expect(turnPhrase('WHATEVER'), 'WHATEVER'); // 모르는 값은 그대로 보여 준다
  });

  group('도보 구간', () {
    final plan = it([
      mk('WALK', steps: walkSteps, toName: '강남(2호선)'),
      mk('SUBWAY', route: '2호선', headsign: '성수', transit: true, fromName: '강남(2호선)', toName: '성수'),
    ]);

    test('지금 할 일은 실시간 남은 거리, 발화는 단계 고정 거리', () {
      final i = buildInstruction(
          request: request, itinerary: plan, legIndex: 0, stepIndex: 0, stepRemainM: 83);
      expect(i.now, '테헤란로 따라 80m 직진 후 우회전');
      expect(i.utterance, '테헤란로 따라 120m 직진 후 우회전');
      expect(i.next, '탑승 · 2호선 성수 방면 · 강남(2호선)');
      // 같은 단계에서 거리가 줄어도 발화 문장은 그대로다(되풀이해 읽지 않게)
      final closer = buildInstruction(
          request: request, itinerary: plan, legIndex: 0, stepIndex: 0, stepRemainM: 41);
      expect(closer.now, '테헤란로 따라 40m 직진 후 우회전');
      expect(closer.utterance, i.utterance);
    });

    test('이름 없는 길은 도로명을 빼고, 10m 미만은 "잠시"', () {
      final i = buildInstruction(
          request: request, itinerary: plan, legIndex: 0, stepIndex: 1, stepRemainM: 6);
      expect(i.now, '잠시 직진 후 강남 8번 출구로 들어가기');
    });

    test('마지막 단계는 도착 문구', () {
      final i = buildInstruction(
          request: request, itinerary: plan, legIndex: 0, stepIndex: 2, stepRemainM: 0);
      expect(i.now, '잠시 직진 하면 강남(2호선) 도착');
    });

    test('단계가 없으면(역 안 통로) 구간 요약을 쓴다', () {
      final bare = it([mk('WALK', durationSec: 180, toName: '사당(2호선)'), mk('SUBWAY', transit: true)]);
      final i = buildInstruction(request: request, itinerary: bare, legIndex: 0);
      expect(i.now, '사당(2호선)까지 도보 3분');
      expect(i.utterance, i.now);
    });

    test('자전거는 "주행"으로 읽는다', () {
      final bike = it([mk('BICYCLE', rented: true, steps: walkSteps, toName: '대여소')]);
      final i = buildInstruction(
          request: request, itinerary: bike, legIndex: 0, stepIndex: 0, stepRemainM: 124);
      expect(i.now, '테헤란로 따라 120m 주행 후 우회전');
      expect(i.next, '도착 · 회사');
    });
  });

  group('대중교통 구간', () {
    final transfer = it([
      mk('SUBWAY', route: '4호선', headsign: '당고개', transit: true, toName: '동대문'),
      mk('WALK', durationSec: 120, fromName: '동대문', toName: '동대문'),
      mk('SUBWAY', route: '1호선', headsign: '소요산', transit: true, fromName: '동대문', toName: '종로3가'),
    ]);

    test('남은 정거장과 다음 정차를 보여 주고, 탑승 문장을 한 번 읽는다', () {
      final i = buildInstruction(
          request: request, itinerary: transfer, legIndex: 0, remainingStops: 3, nextStopName: '혜화');
      expect(i.now, '3정거장 뒤 동대문에서 내리기 · 다음 정차 혜화');
      expect(i.utterance, '4호선 당고개 방면을 타고 3정거장 뒤 동대문에서 내리세요');
      expect(i.next, '환승 · 1호선 소요산 방면 · 동대문');
      // 정거장이 줄어도 발화는 같은 문장을 유지한다(중복 제거로 다시 읽히지 않는다)
      final closer = buildInstruction(
          request: request, itinerary: transfer, legIndex: 0, remainingStops: 3, nextStopName: '동묘앞');
      expect(closer.utterance, i.utterance);
    });

    test('한 정거장 남으면 하차 안내와 다음 할 일을 함께 읽는다', () {
      final i = buildInstruction(request: request, itinerary: transfer, legIndex: 0, remainingStops: 1);
      expect(i.now, '다음 역에서 내리세요 · 동대문');
      expect(i.utterance, '다음 역에서 내리세요. 환승 · 1호선 소요산 방면 · 동대문');
    });

    test('버스는 "정류장", 내린 뒤 도보면 출구 안내', () {
      final busThenWalk = it([
        mk('BUS', route: '402', headsign: '서울역', transit: true, toName: '숭례문'),
        mk('WALK', toName: '회사', steps: const [
          WalkStep(dir: 'DEPART', distanceM: 30, lat: 37.5, lon: 127.0),
          WalkStep(dir: 'EXIT_STATION', entrance: '시청 4번 출구', distanceM: 0, lat: 37.501, lon: 127.0),
        ]),
      ]);
      final i = buildInstruction(request: request, itinerary: busThenWalk, legIndex: 0, remainingStops: 1);
      expect(i.now, '다음 정류장에서 내리세요 · 숭례문');
      expect(i.next, '내려서 시청 4번 출구로 나가기');
    });

    test('버스에서 내린 뒤에는 출구 대신 다음 목적지를 안내한다', () {
      final plain = it([
        mk('BUS', route: '402', transit: true, toName: '숭례문'),
        mk('WALK', toName: '회사'),
      ]);
      final i = buildInstruction(request: request, itinerary: plain, legIndex: 0, remainingStops: 1);
      expect(i.next, '회사까지 도보'); // 버스 정류장에는 출구가 없다
      expect(i.utterance, '다음 정류장에서 내리세요. 회사까지 도보');
    });
  });

  test('따릉이 대여·반납이 다음 할 일에 나온다', () {
    final bike = it([
      mk('WALK', steps: walkSteps, toName: '대여소'),
      mk('BICYCLE', rented: true, steps: walkSteps, toName: '반납소'),
      mk('WALK', steps: walkSteps, toName: '회사'),
    ]);
    expect(
        buildInstruction(request: request, itinerary: bike, legIndex: 0, stepIndex: 0, stepRemainM: 10).next,
        '따릉이 대여 · 반납소');
    expect(
        buildInstruction(request: request, itinerary: bike, legIndex: 1, stepIndex: 0, stepRemainM: 10).next,
        '따릉이 반납 · 회사');
  });
}
