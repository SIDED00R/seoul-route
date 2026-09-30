import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/instruction.dart';
import 'package:seoul_route/models/fast_exit.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/leg_detail.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';

const request = PlanRequest(
  origin: Place(name: '집 앞', address: '', lat: 37.5, lon: 127.0),
  destination: Place(name: '회사', address: '', lat: 37.52, lon: 127.04),
);

const exitSide = [
  FastExitFacility(name: '에스컬레이터', doors: ['3-3', '8-1', '9-4']),
  FastExitFacility(name: '계단', doors: ['2-1']),
  FastExitFacility(name: '환승통로 계단', doors: ['5-2']),
];

Leg leg(String mode, {bool transit = false, String route = '', String headsign = '', String toName = 'b',
        List<TransitStop> stops = const [], List<FastExitFacility> fastExit = const []}) =>
    Leg(
      mode: mode,
      durationSec: 300,
      distanceM: 300,
      fromName: 'a',
      toName: toName,
      fromLat: 37.5,
      fromLon: 127.0,
      toLat: 37.502,
      toLon: 127.0,
      route: route,
      rentedBike: false,
      transitLeg: transit,
      polyline: '',
      start: '',
      end: '',
      headsign: headsign,
      stops: stops,
      fastExit: fastExit,
    );

Itinerary it(List<Leg> legs) =>
    Itinerary(start: '', end: '', durationSec: 1800, transfers: 0, walkM: 500, legs: legs);

const twoStops = [TransitStop(name: '성수', lat: 37.5, lon: 127.05, stopId: '', offsetSec: 120)];

Leg line2({List<FastExitFacility> fastExit = exitSide, List<TransitStop> stops = twoStops}) => leg('SUBWAY',
    transit: true, route: '2호선', headsign: '성수', toName: '잠실', stops: stops, fastExit: fastExit);

void main() {
  test('탑승 문장에 출구 쪽 설비의 칸-문을 앞에서 두 개까지 붙인다', () {
    final i = buildInstruction(
        request: request, itinerary: it([line2(), leg('WALK', toName: '회사')]), legIndex: 0, remainingStops: 2);
    expect(i.cueKey, 'L0:board');
    expect(i.utterance,
        '2호선 성수 방면을 타고 2정거장 뒤 잠실에서 내리세요. 에스컬레이터와 가까운 3번 칸 3번 문이나 8번 칸 1번 문 쪽에서 타세요');
  });

  test('내린 뒤 지하철로 갈아타면 환승통로 설비를 고른다(받침 있으면 "과")', () {
    final i = buildInstruction(
      request: request,
      itinerary: it([
        line2(),
        leg('WALK', toName: '잠실'),
        leg('SUBWAY', transit: true, route: '8호선', headsign: '모란'),
      ]),
      legIndex: 0,
      remainingStops: 2,
    );
    expect(i.utterance, endsWith('. 환승통로 계단과 가까운 5번 칸 2번 문 쪽에서 타세요'));
  });

  test('버스로 갈아타면 환승통로가 아니라 출구 쪽 설비, 그 종류가 없으면 첫 설비', () {
    final toBus = buildInstruction(
      request: request,
      itinerary: it([line2(), leg('WALK'), leg('BUS', transit: true, route: '402')]),
      legIndex: 0,
      remainingStops: 2,
    );
    expect(toBus.utterance, contains('에스컬레이터와 가까운 3번 칸 3번 문'));
    final onlyTransfer = buildInstruction(
      request: request,
      itinerary: it([line2(fastExit: const [FastExitFacility(name: '환승통로 에스컬레이터', doors: ['1-1'])]), leg('WALK')]),
      legIndex: 0,
      remainingStops: 2,
    );
    expect(onlyTransfer.utterance, endsWith('. 환승통로 에스컬레이터와 가까운 1번 칸 1번 문 쪽에서 타세요'));
  });

  test('두 문 사이 표기("3-2,3-3 사이")도 칸·문으로 읽고 조사를 맞춘다', () {
    String phrase(List<String> doors) =>
        fastExitPhrase(line2(fastExit: [FastExitFacility(name: '계단', doors: doors)]), transferAfter: false);
    expect(phrase(['3-2,3-3 사이', '10-2']), '계단과 가까운 3번 칸 2번 문과 3번 칸 3번 문 사이나 10번 칸 2번 문 쪽에서 타세요');
    expect(phrase(['10-2', '3-4,4-1 사이']), '계단과 가까운 10번 칸 2번 문이나 3번 칸 4번 문과 4번 칸 1번 문 사이 쪽에서 타세요');
  });

  test('자료가 없으면 탑승 문장은 그대로다', () {
    final i = buildInstruction(
        request: request, itinerary: it([line2(fastExit: const []), leg('WALK')]), legIndex: 0, remainingStops: 2);
    expect(i.utterance, '2호선 성수 방면을 타고 2정거장 뒤 잠실에서 내리세요');
  });

  test('한 정거장짜리 구간은 탑승·빠른 하차·하차·다음 할 일을 한 문장으로', () {
    final i = buildInstruction(
        request: request, itinerary: it([line2(stops: const []), leg('WALK', toName: '회사')]), legIndex: 0,
        remainingStops: 1);
    expect(i.cueKey, 'L0:alight');
    expect(i.utterance, '2호선 성수 방면을 타고 다음 역에서 내리세요. '
        '에스컬레이터와 가까운 3번 칸 3번 문이나 8번 칸 1번 문 쪽에서 타세요. 회사까지 도보');
  });

  test('이미 타고 가다 한 정거장 남은 하차 안내에는 붙이지 않는다', () {
    final i = buildInstruction(
        request: request, itinerary: it([line2(), leg('WALK', toName: '회사')]), legIndex: 0, remainingStops: 1);
    expect(i.utterance, '다음 역에서 내리세요. 회사까지 도보');
  });

  test('화면·알림 문구: 탑승 전 "다음" 안내와 타고 가는 동안의 현재 안내에 짧은 칸-문 표기를 붙인다', () {
    final before = buildInstruction(
        request: request, itinerary: it([leg('WALK'), line2(), leg('WALK', toName: '회사')]), legIndex: 0);
    expect(before.next, '탑승 · 2호선 성수 방면 · a · 에스컬레이터 3-3, 8-1 쪽 탑승');
    final riding = buildInstruction(
        request: request, itinerary: it([leg('WALK'), line2(), leg('WALK', toName: '회사')]), legIndex: 1,
        remainingStops: 2, nextStopName: '역삼');
    expect(riding.now, '2정거장 뒤 잠실에서 내리기 · 다음 정차 역삼 · 에스컬레이터 3-3, 8-1 쪽 탑승');
    final transfer = buildInstruction(
      request: request,
      itinerary: it([
        line2(),
        leg('WALK', toName: '잠실'),
        leg('SUBWAY', transit: true, route: '8호선', headsign: '모란',
            fastExit: const [FastExitFacility(name: '계단', doors: ['1-1', '2-2', '3-3'])]),
      ]),
      legIndex: 0,
      remainingStops: 2,
    );
    expect(transfer.next, endsWith(' · 계단 1-1, 2-2 쪽 탑승'));
    final none = buildInstruction(
        request: request, itinerary: it([leg('WALK'), line2(fastExit: const []), leg('WALK')]), legIndex: 0);
    expect(none.next, '탑승 · 2호선 성수 방면 · a');
  });

  test('짧은 칸-문 표기는 읽는 문장에는 들어가지 않는다', () {
    final i = buildInstruction(
      request: request,
      itinerary: it([
        line2(),
        leg('WALK', toName: '잠실'),
        leg('SUBWAY', transit: true, route: '8호선', headsign: '모란',
            fastExit: const [FastExitFacility(name: '계단', doors: ['1-1', '2-2'])]),
      ]),
      legIndex: 0,
      remainingStops: 1,
    );
    expect(i.next, endsWith(' · 계단 1-1, 2-2 쪽 탑승'));
    expect(i.utterance, '다음 역에서 내리세요. 환승 · 8호선 모란 방면 · a');
  });

  test('한 정거장짜리 구간은 승강장에서 기다리는 현재 안내에도 짧은 칸-문 표기를 붙인다', () {
    final i = buildInstruction(
        request: request, itinerary: it([leg('WALK'), line2(stops: const []), leg('WALK', toName: '회사')]),
        legIndex: 1, remainingStops: 1);
    expect(i.now, '다음 역에서 내리세요 · 잠실 · 에스컬레이터 3-3, 8-1 쪽 탑승');
    final riding = buildInstruction(
        request: request, itinerary: it([leg('WALK'), line2(), leg('WALK', toName: '회사')]), legIndex: 1,
        remainingStops: 1);
    expect(riding.now, '다음 역에서 내리세요 · 잠실');
  });
}
