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
}) => Leg(
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
  WalkStep(
    dir: 'DEPART',
    street: '테헤란로',
    distanceM: 124,
    lat: 37.5,
    lon: 127.0,
  ),
  WalkStep(dir: 'RIGHT', distanceM: 46, lat: 37.501, lon: 127.0),
  WalkStep(
    dir: 'ENTER_STATION',
    entrance: '강남 8번 출구',
    distanceM: 0,
    lat: 37.5015,
    lon: 127.0,
  ),
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
      mk(
        'SUBWAY',
        route: '2호선',
        headsign: '성수',
        transit: true,
        fromName: '강남(2호선)',
        toName: '성수',
      ),
    ]);

    test('지금 할 일은 실시간 남은 거리, 발화는 단계 고정 거리', () {
      final i = buildInstruction(
        request: request,
        itinerary: plan,
        legIndex: 0,
        stepIndex: 0,
        stepRemainM: 83,
      );
      expect(i.now, '80m 직진 후 우회전');
      expect(i.utterance, '120m 직진 후 우회전');
      expect(i.next, '탑승 · 2호선 성수 방면 · 강남(2호선)');
      // 같은 단계에서 거리가 줄어도 발화 문장은 그대로다(되풀이해 읽지 않게)
      final closer = buildInstruction(
        request: request,
        itinerary: plan,
        legIndex: 0,
        stepIndex: 0,
        stepRemainM: 41,
      );
      expect(closer.now, '40m 직진 후 우회전');
      expect(closer.utterance, i.utterance);
    });

    test('이름 없는 길은 도로명을 빼고, 10m 미만은 "잠시"', () {
      final i = buildInstruction(
        request: request,
        itinerary: plan,
        legIndex: 0,
        stepIndex: 1,
        stepRemainM: 6,
      );
      expect(i.now, '잠시 직진 후 강남 8번 출구로 들어가기');
    });

    test('마지막 단계는 도착 문구, 출입구 단계면 그 동작을 앞에 붙인다', () {
      final i = buildInstruction(
        request: request,
        itinerary: plan,
        legIndex: 0,
        stepIndex: 2,
        stepRemainM: 0,
      );
      expect(i.now, '강남 8번 출구로 들어가서 잠시 직진 하면 강남(2호선) 도착');
    });

    test('단계가 없으면(역 안 통로) 구간 요약을 쓴다', () {
      final bare = it([
        mk('WALK', durationSec: 180, toName: '사당(2호선)'),
        mk('SUBWAY', transit: true),
      ]);
      final i = buildInstruction(
        request: request,
        itinerary: bare,
        legIndex: 0,
      );
      expect(i.now, '사당(2호선)까지 도보 3분');
      expect(i.utterance, i.now);
    });

    test('자전거는 "주행"으로 읽는다', () {
      final bike = it([
        mk('BICYCLE', rented: true, steps: walkSteps, toName: '대여소'),
      ]);
      final i = buildInstruction(
        request: request,
        itinerary: bike,
        legIndex: 0,
        stepIndex: 0,
        stepRemainM: 124,
      );
      expect(i.now, '120m 주행 후 우회전');
      expect(i.next, '도착 · 회사');
    });
  });

  group('대중교통 구간', () {
    final transfer = it([
      mk(
        'SUBWAY',
        route: '4호선',
        headsign: '당고개',
        transit: true,
        toName: '동대문',
        stops: const [
          TransitStop(name: '혜화', lat: 37.5, lon: 127.0),
          TransitStop(name: '동묘앞', lat: 37.5, lon: 127.0),
        ],
      ),
      mk('WALK', durationSec: 120, fromName: '동대문', toName: '동대문'),
      mk(
        'SUBWAY',
        route: '1호선',
        headsign: '소요산',
        transit: true,
        fromName: '동대문',
        toName: '종로3가',
      ),
    ]);

    test('남은 정거장과 다음 정차를 보여 주고, 탑승 문장을 한 번 읽는다', () {
      final i = buildInstruction(
        request: request,
        itinerary: transfer,
        legIndex: 0,
        remainingStops: 3,
        nextStopName: '혜화',
      );
      expect(i.now, '3정거장 뒤 동대문에서 내리기 · 다음 정차 혜화');
      expect(i.utterance, '4호선 당고개 방면을 타고 3정거장 뒤 동대문에서 내리세요');
      expect(i.next, '환승 · 1호선 소요산 방면 · 동대문');
      expect(i.cueKey, 'L0:board');
      // 역을 지나 남은 정거장이 줄어도 같은 안내 시점이고 문장도 그대로다 — 역마다 다시 읽지 않는다.
      final closer = buildInstruction(
        request: request,
        itinerary: transfer,
        legIndex: 0,
        remainingStops: 2,
        nextStopName: '동묘앞',
      );
      expect(closer.now, '2정거장 뒤 동대문에서 내리기 · 다음 정차 동묘앞');
      expect(closer.cueKey, i.cueKey);
      expect(closer.utterance, i.utterance);
    });

    test('한 정거장 남으면 하차 안내와 다음 할 일을 함께 읽는다', () {
      final i = buildInstruction(
        request: request,
        itinerary: transfer,
        legIndex: 0,
        remainingStops: 1,
      );
      expect(i.now, '다음 역에서 내리세요 · 동대문');
      expect(i.utterance, '다음 역에서 내리세요. 환승 · 1호선 소요산 방면 · 동대문');
      expect(i.cueKey, 'L0:alight');
    });

    test('한 정거장짜리 구간은 탑승과 하차를 한 문장으로 읽는다', () {
      final oneStop = it([
        mk(
          'SUBWAY',
          route: '4호선',
          headsign: '오이도',
          transit: true,
          toName: '총신대입구(이수)',
        ),
        mk('WALK', toName: '회사'),
      ]);
      final i = buildInstruction(
        request: request,
        itinerary: oneStop,
        legIndex: 0,
        remainingStops: 1,
      );
      expect(i.utterance, '4호선 오이도 방면을 타고 다음 역에서 내리세요. 회사까지 도보');
    });

    test('버스는 "정류장", 내린 뒤 도보면 출구 안내', () {
      final busThenWalk = it([
        mk('BUS', route: '402', headsign: '서울역', transit: true, toName: '숭례문'),
        mk(
          'WALK',
          toName: '회사',
          steps: const [
            WalkStep(dir: 'DEPART', distanceM: 30, lat: 37.5, lon: 127.0),
            WalkStep(
              dir: 'EXIT_STATION',
              entrance: '시청 4번 출구',
              distanceM: 0,
              lat: 37.501,
              lon: 127.0,
            ),
          ],
        ),
      ]);
      final i = buildInstruction(
        request: request,
        itinerary: busThenWalk,
        legIndex: 0,
        remainingStops: 1,
      );
      expect(i.now, '다음 정류장에서 내리세요 · 숭례문');
      expect(i.next, '내려서 시청 4번 출구로 나가기');
    });

    test('버스에서 내린 뒤에는 출구 대신 다음 목적지를 안내한다', () {
      final plain = it([
        mk('BUS', route: '402', transit: true, toName: '숭례문'),
        mk('WALK', toName: '회사'),
      ]);
      final i = buildInstruction(
        request: request,
        itinerary: plain,
        legIndex: 0,
        remainingStops: 1,
      );
      expect(i.next, '회사까지 도보'); // 버스 정류장에는 출구가 없다
      expect(i.utterance, '버스 402를 타고 다음 정류장에서 내리세요. 회사까지 도보');
    });
  });

  test('목적격 조사: 받침이 있으면 을, 없으면 를(숫자는 읽는 소리)', () {
    expect(withObjectParticle('2호선 성수 방면'), '2호선 성수 방면을');
    expect(withObjectParticle('버스 402'), '버스 402를'); // 이
    expect(withObjectParticle('버스 470'), '버스 470을'); // 영
    expect(withObjectParticle('버스 9401'), '버스 9401을'); // 일
    expect(withObjectParticle('공항철도'), '공항철도를');
  });

  test('이어지는 직진 단계는 다음 회전까지 한 안내로 합친다', () {
    final walk = it([
      mk(
        'WALK',
        toName: '역',
        steps: const [
          WalkStep(dir: 'DEPART', distanceM: 96, lat: 37.5, lon: 127.0),
          WalkStep(dir: 'CONTINUE', distanceM: 19, lat: 37.5009, lon: 127.0),
          WalkStep(
            dir: 'CONTINUE',
            street: '서울역광장',
            distanceM: 7,
            lat: 37.501,
            lon: 127.0,
          ),
          WalkStep(dir: 'LEFT', distanceM: 32, lat: 37.5011, lon: 127.0),
        ],
      ),
    ]);
    final first = buildInstruction(
      request: request,
      itinerary: walk,
      legIndex: 0,
      stepIndex: 0,
      stepRemainM: 96,
    );
    expect(first.now, '120m 직진 후 좌회전'); // 96 + 19 + 7
    expect(first.cueKey, 'L0:T3');
    // 직진 단계로 넘어가도 향하는 회전이 같으면 같은 안내 시점이다(다시 읽지 않는다)
    final second = buildInstruction(
      request: request,
      itinerary: walk,
      legIndex: 0,
      stepIndex: 1,
      stepRemainM: 10,
    );
    expect(second.now, '20m 직진 후 좌회전'); // 10 + 7
    expect(second.cueKey, first.cueKey);
    final last = buildInstruction(
      request: request,
      itinerary: walk,
      legIndex: 0,
      stepIndex: 3,
      stepRemainM: 32,
    );
    expect(last.cueKey, 'L0:arrive');
  });

  test('랜드마크가 있으면 도로명 대신 회전 기준점으로 읽는다', () {
    final walk = it([mk('WALK', steps: walkSteps, toName: '역')]);
    final i = buildInstruction(
      request: request,
      itinerary: walk,
      legIndex: 0,
      stepIndex: 0,
      stepRemainM: 83,
      landmark: '우리은행 목동점',
    );
    expect(i.now, '80m 직진 후 우리은행 목동점 근처에서 우회전');
    expect(i.utterance, '120m 직진 후 우리은행 목동점 근처에서 우회전');
    expect(nextTurnStepIndex(walkSteps, 0), 1);
  });

  test('따릉이 대여·반납이 다음 할 일에 나온다', () {
    final bike = it([
      mk('WALK', steps: walkSteps, toName: '대여소'),
      mk('BICYCLE', rented: true, steps: walkSteps, toName: '반납소'),
      mk('WALK', steps: walkSteps, toName: '회사'),
    ]);
    expect(
      buildInstruction(
        request: request,
        itinerary: bike,
        legIndex: 0,
        stepIndex: 0,
        stepRemainM: 10,
      ).next,
      '따릉이 대여 · 반납소',
    );
    expect(
      buildInstruction(
        request: request,
        itinerary: bike,
        legIndex: 1,
        stepIndex: 0,
        stepRemainM: 10,
      ).next,
      '따릉이 반납 · 회사',
    );
  });
}
