import '../models/itinerary.dart';
import '../models/leg_detail.dart';
import '../models/plan_request.dart';
import '../util/leg_names.dart';

/// 안내 카드에 넣을 문구. now 는 실시간 남은 거리·정거장 수가 들어가 매 위치마다 바뀐다. utterance 는 읽어 줄 문장이고
/// cueKey 는 그 안내 시점의 이름(탑승·하차·몇 번째 회전)이다 — VoiceGuide 는 같은 cueKey 를 한 번만 읽는다.
class Instruction {
  const Instruction({
    required this.now,
    required this.next,
    this.utterance,
    this.cueKey = '',
  });

  final String now;
  final String next;
  final String? utterance;
  final String cueKey;
}

/// 회전 방향 문구(OTP relativeDirection). entrance 는 역 출입구 이름, exit 은 회전교차로 출구 번호.
String turnPhrase(String dir, {String entrance = '', String exit = ''}) {
  switch (dir) {
    case 'DEPART':
      return '출발';
    case 'CONTINUE':
      return '직진';
    case 'SLIGHTLY_LEFT':
      return '왼쪽으로 약간';
    case 'SLIGHTLY_RIGHT':
      return '오른쪽으로 약간';
    case 'LEFT':
      return '좌회전';
    case 'RIGHT':
      return '우회전';
    case 'HARD_LEFT':
      return '왼쪽으로 크게';
    case 'HARD_RIGHT':
      return '오른쪽으로 크게';
    case 'UTURN_LEFT':
    case 'UTURN_RIGHT':
      return '유턴';
    case 'CIRCLE_CLOCKWISE':
    case 'CIRCLE_COUNTERCLOCKWISE':
      return exit.isEmpty ? '회전교차로 진입' : '회전교차로 $exit번 출구';
    case 'ELEVATOR':
      return '엘리베이터 이용';
    case 'ENTER_STATION':
      return entrance.isEmpty ? '역으로 들어가기' : '$entrance로 들어가기';
    case 'EXIT_STATION':
      return entrance.isEmpty ? '역에서 나가기' : '$entrance로 나가기';
    case 'FOLLOW_SIGNS':
      return '표지판 따라가기';
    default:
      return dir;
  }
}

/// 거리 문구. 10m 단위로 끊고, 그보다 짧으면 "잠시".
String _distText(double m) => m < 10 ? '잠시' : '${(m / 10).round() * 10}m';

String _go(double distM, {required bool bike}) {
  final verb = bike ? '주행' : '직진';
  return '${_distText(distM)} $verb';
}

/// 현재 단계부터 이어지는 CONTINUE 를 건너뛴 다음 실제 행동 단계. 없으면 -1이다.
int nextTurnStepIndex(List<WalkStep> steps, int stepIndex) {
  var j = stepIndex.clamp(0, steps.isEmpty ? 0 : steps.length - 1) + 1;
  while (j < steps.length && steps[j].dir == 'CONTINUE') {
    j++;
  }
  return j < steps.length ? j : -1;
}

/// 안내 카드 문구를 만든다. stepRemainM 은 현재 단계의 남은 거리(도보·자전거), remainingStops 는 하차까지
/// 남은 정거장 수(하차역 포함, 대중교통).
Instruction buildInstruction({
  required PlanRequest request,
  required Itinerary itinerary,
  required int legIndex,

  /// 경로 이탈 재탐색으로 갈아 낀 구간 목록. 없으면 itinerary.legs.
  List<Leg>? legs,
  int stepIndex = 0,
  double? stepRemainM,
  int remainingStops = 0,
  String? nextStopName,
  String landmark = '',
}) {
  final ls = legs ?? itinerary.legs;
  final leg = ls[legIndex];
  final next = _nextAction(request, ls, legIndex);
  if (leg.transitLeg) {
    final to = legEndpointName(request, leg.toName, leg.toLat, leg.toLon);
    final unit = leg.mode == 'BUS' ? '정류장' : '역';
    final ride = '${leg.label}${_headsignText(leg)}';
    final total = leg.stops.length + 1; // 하차역 포함 전체 정거장 수
    if (remainingStops <= 1) {
      // 한 정거장짜리 구간은 탑승 안내 없이 바로 여기로 오므로 무엇을 타는지도 함께 읽는다.
      final head = total <= 1 ? '${withObjectParticle(ride)} 타고 ' : '';
      return Instruction(
        now: '다음 $unit에서 내리세요 · $to',
        next: next,
        utterance: '$head다음 $unit에서 내리세요. $next',
        cueKey: 'L$legIndex:alight',
      );
    }
    final via = nextStopName == null ? '' : ' · 다음 정차 $nextStopName';
    return Instruction(
      now: '$remainingStops정거장 뒤 $to에서 내리기$via',
      next: next,
      // 탑승 안내는 구간에 들어올 때 한 번만 읽는다. 문장의 정거장 수는 구간 전체 값이라 역을 지나도 바뀌지 않는다.
      utterance: '${withObjectParticle(ride)} 타고 $total정거장 뒤 $to에서 내리세요',
      cueKey: 'L$legIndex:board',
    );
  }
  final to = legEndpointName(request, leg.toName, leg.toLat, leg.toLon);
  if (leg.steps.isEmpty) {
    final mins = (leg.durationSec / 60).round();
    final text = '$to까지 ${leg.label} $mins분';
    return Instruction(
      now: text,
      next: next,
      utterance: text,
      cueKey: 'L$legIndex:walk',
    );
  }
  final i = stepIndex.clamp(0, leg.steps.length - 1);
  final step = leg.steps[i];
  final bike = leg.mode == 'BICYCLE';
  // 다음 "실제 회전"까지 이어지는 직진 단계들은 한 안내로 합친다(OTP 는 길 이름·종류가 바뀔 때마다 CONTINUE 를 낸다).
  var j = i + 1;
  var ahead = 0.0; // 현재 단계 뒤, 회전 전까지의 직진 단계 거리 합
  while (j < leg.steps.length && leg.steps[j].dir == 'CONTINUE') {
    final s = leg.steps[j];
    ahead += s.distanceM;
    j++;
  }
  final live = (stepRemainM ?? step.distanceM) + ahead;
  final fixed = step.distanceM + ahead;
  final lead = _actionLead(step);
  if (j >= leg.steps.length) {
    return Instruction(
      now: '$lead${_go(live, bike: bike)} 하면 $to 도착',
      next: next,
      utterance: '$lead${_go(fixed, bike: bike)} 하면 $to 도착',
      cueKey: 'L$legIndex:arrive',
    );
  }
  final after = leg.steps[j];
  final turn = turnPhrase(
    after.dir,
    entrance: after.entrance,
    exit: after.exit,
  );
  final cue = landmark.isEmpty ? turn : '$landmark 근처에서 $turn';
  return Instruction(
    now: '$lead${_go(live, bike: bike)} 후 $cue',
    next: next,
    utterance: '$lead${_go(fixed, bike: bike)} 후 $cue',
    cueKey: 'L$legIndex:T$j', // 같은 회전을 향하는 동안은 단계가 바뀌어도 같은 안내다
  );
}

/// 현재 단계가 그 자리에서 하는 동작(출입구·엘리베이터)이면 문장 앞에 붙일 말. 그 단계의 거리는 0 인 경우가 많아
/// 뒤따르는 직진 안내만 내면 동작이 빠진다.
String _actionLead(WalkStep step) {
  switch (step.dir) {
    case 'EXIT_STATION':
      return step.entrance.isEmpty ? '역에서 나가서 ' : '${step.entrance}로 나가서 ';
    case 'ENTER_STATION':
      return step.entrance.isEmpty ? '역으로 들어가서 ' : '${step.entrance}로 들어가서 ';
    case 'ELEVATOR':
      return '엘리베이터를 타고 ';
    default:
      return '';
  }
}

String _headsignText(Leg leg) =>
    leg.headsign.isEmpty ? '' : ' ${leg.headsign} 방면';

/// 목적격 조사를 붙인다("2호선을"·"버스 402를"). 받침이 있으면 을, 없으면 를. 숫자는 읽는 소리로 가린다.
String withObjectParticle(String word) {
  if (word.isEmpty) return word;
  final c = word.codeUnitAt(word.length - 1);
  bool hasFinal;
  if (c >= 0xAC00 && c <= 0xD7A3) {
    hasFinal = (c - 0xAC00) % 28 != 0;
  } else if (c >= 0x30 && c <= 0x39) {
    hasFinal = '013678'.contains(String.fromCharCode(c)); // 영·일·삼·육·칠·팔
  } else {
    hasFinal = false;
  }
  return '$word${hasFinal ? '을' : '를'}';
}

/// 이 구간 다음에 할 일. 환승·하차 후 출구·대여·반납·도착.
String _nextAction(PlanRequest request, List<Leg> legs, int legIndex) {
  if (legIndex + 1 >= legs.length) return '도착 · ${request.destination.name}';
  final next = legs[legIndex + 1];
  final to = legEndpointName(request, next.toName, next.toLat, next.toLon);
  if (next.transitLeg) {
    final board = legEndpointName(
      request,
      next.fromName,
      next.fromLat,
      next.fromLon,
    );
    final verb = legs[legIndex].transitLeg ? '환승' : '탑승';
    return '$verb · ${next.label}${_headsignText(next)} · $board';
  }
  if (next.mode == 'BICYCLE') {
    return '${next.rentedBike ? '따릉이 대여' : '자전거 타기'} · $to';
  }
  if (legs[legIndex].mode == 'BICYCLE') {
    return '${legs[legIndex].rentedBike ? '따릉이 반납' : '자전거 세우기'} · $to';
  }
  if (legs[legIndex].transitLeg) {
    // 내린 뒤 도보가 다음 탑승으로 이어지면(역 안 환승 통로·정류장 간 도보) 환승으로 안내한다.
    if (legIndex + 2 < legs.length && legs[legIndex + 2].transitLeg) {
      final ride = legs[legIndex + 2];
      final board = legEndpointName(
        request,
        ride.fromName,
        ride.fromLat,
        ride.fromLon,
      );
      return '환승 · ${ride.label}${_headsignText(ride)} · $board';
    }
    // 출구 안내는 역 출입구를 실제로 지날 때만 한다(버스 정류장에는 출구가 없다).
    final exit = _firstEntrance(next.steps);
    if (exit != null) return '내려서 $exit로 나가기';
  }
  return '$to까지 ${next.label}';
}

/// 도보 단계에서 처음 만나는 역 출입구 이름(없으면 null).
String? _firstEntrance(List<WalkStep> steps) {
  for (final s in steps) {
    if (s.entrance.isNotEmpty) return s.entrance;
  }
  return null;
}
