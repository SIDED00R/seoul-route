import '../models/itinerary.dart';
import '../models/leg_detail.dart';
import '../models/plan_request.dart';
import '../util/leg_names.dart';

/// 안내 카드에 넣을 문구. now 는 실시간 남은 거리가 들어가 매 위치마다 바뀌고, utterance 는 단계마다 고정이라
/// 같은 단계에서는 한 번만 읽힌다(VoiceGuide 가 직전 문장과 같으면 건너뛴다).
class Instruction {
  const Instruction({required this.now, required this.next, this.utterance});

  final String now;
  final String next;
  final String? utterance;
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

String _go(WalkStep step, double distM, {required bool bike}) {
  final verb = bike ? '주행' : '직진';
  final head = step.street.isEmpty ? '' : '${step.street} 따라 ';
  return '$head${_distText(distM)} $verb';
}

/// 안내 카드 문구를 만든다. stepRemainM 은 현재 단계의 남은 거리(도보·자전거), remainingStops 는 하차까지
/// 남은 정거장 수(하차역 포함, 대중교통).
Instruction buildInstruction({
  required PlanRequest request,
  required Itinerary itinerary,
  required int legIndex,
  int stepIndex = 0,
  double? stepRemainM,
  int remainingStops = 0,
  String? nextStopName,
}) {
  final legs = itinerary.legs;
  final leg = legs[legIndex];
  final next = _nextAction(request, itinerary, legIndex);
  if (leg.transitLeg) {
    final to = legEndpointName(request, leg.toName, leg.toLat, leg.toLon);
    final unit = leg.mode == 'BUS' ? '정류장' : '역';
    if (remainingStops <= 1) {
      return Instruction(
        now: '다음 $unit에서 내리세요 · $to',
        next: next,
        utterance: '다음 $unit에서 내리세요. $next',
      );
    }
    final via = nextStopName == null ? '' : ' · 다음 정차 $nextStopName';
    return Instruction(
      now: '$remainingStops정거장 뒤 $to에서 내리기$via',
      next: next,
      utterance: '${leg.label}${_headsignText(leg)}을 타고 $remainingStops정거장 뒤 $to에서 내리세요',
    );
  }
  final to = legEndpointName(request, leg.toName, leg.toLat, leg.toLon);
  if (leg.steps.isEmpty) {
    final mins = (leg.durationSec / 60).round();
    final text = '$to까지 ${leg.label} $mins분';
    return Instruction(now: text, next: next, utterance: text);
  }
  final i = stepIndex.clamp(0, leg.steps.length - 1);
  final step = leg.steps[i];
  final bike = leg.mode == 'BICYCLE';
  if (i + 1 >= leg.steps.length) {
    final go = _go(step, stepRemainM ?? step.distanceM, bike: bike);
    return Instruction(
      now: '$go 하면 $to 도착',
      next: next,
      utterance: '${_go(step, step.distanceM, bike: bike)} 하면 $to 도착',
    );
  }
  final after = leg.steps[i + 1];
  final turn = turnPhrase(after.dir, entrance: after.entrance, exit: after.exit);
  return Instruction(
    now: '${_go(step, stepRemainM ?? step.distanceM, bike: bike)} 후 $turn',
    next: next,
    utterance: '${_go(step, step.distanceM, bike: bike)} 후 $turn',
  );
}

String _headsignText(Leg leg) => leg.headsign.isEmpty ? '' : ' ${leg.headsign} 방면';

/// 이 구간 다음에 할 일. 환승·하차 후 출구·대여·반납·도착.
String _nextAction(PlanRequest request, Itinerary itinerary, int legIndex) {
  final legs = itinerary.legs;
  if (legIndex + 1 >= legs.length) return '도착 · ${request.destination.name}';
  final next = legs[legIndex + 1];
  final to = legEndpointName(request, next.toName, next.toLat, next.toLon);
  if (next.transitLeg) {
    final board = legEndpointName(request, next.fromName, next.fromLat, next.fromLon);
    final verb = legs[legIndex].transitLeg ? '환승' : '탑승';
    return '$verb · ${next.label}${_headsignText(next)} · $board';
  }
  if (next.mode == 'BICYCLE') return '${next.rentedBike ? '따릉이 대여' : '자전거 타기'} · $to';
  if (legs[legIndex].mode == 'BICYCLE') return '${legs[legIndex].rentedBike ? '따릉이 반납' : '자전거 세우기'} · $to';
  if (legs[legIndex].transitLeg) {
    // 내린 뒤 도보가 다음 탑승으로 이어지면(역 안 환승 통로·정류장 간 도보) 환승으로 안내한다.
    if (legIndex + 2 < legs.length && legs[legIndex + 2].transitLeg) {
      final ride = legs[legIndex + 2];
      final board = legEndpointName(request, ride.fromName, ride.fromLat, ride.fromLon);
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
