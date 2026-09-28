import '../models/plan_request.dart';
import '../util/hhmm.dart';
import '../util/stay_label.dart';
import 'instruction.dart';

/// 경유지 체류 중 안내 문구. after 는 체류가 끝나면 이어 할 안내(경유지 다음 구간의 안내)다.
/// 음성은 체류에 들어올 때 한 번 읽는다(cueKey 'L{legIndex}:stay').
Instruction stayInstruction({
  required PlanRequest request,
  required int stayVia,
  required double staySec,
  required DateTime until,
  required DateTime now,
  required int legIndex,
  required Instruction after,
}) {
  final i = stayVia - 1;
  final known = i >= 0 && i < request.via.length && request.via[i].name.isNotEmpty;
  final name = known ? request.via[i].name : '경유지 $stayVia';
  final left = until.difference(now).inSeconds;
  final leftMin = left <= 0 ? 0 : (left / 60).ceil();
  final at = until.toLocal();
  final spokenAt = at.minute == 0 ? '${at.hour}시' : '${at.hour}시 ${at.minute}분';
  return Instruction(
    now: '$name에서 머무는 중 · ${hhmm(at)} 출발 · $leftMin분 남음',
    next: after.now,
    utterance: '$name에 도착했습니다. ${stayLabel((staySec / 60).round())} 머무른 뒤 $spokenAt에 출발합니다',
    cueKey: 'L$legIndex:stay',
  );
}

/// 체류가 끝났을 때 한 번 읽을 안내. after 의 안내 시점(cueKey)으로 읽어 같은 안내를 다시 읽지 않게 한다.
Instruction stayOverInstruction(Instruction after) => Instruction(
      now: after.now,
      next: after.next,
      utterance: after.utterance == null ? '출발할 시간입니다' : '출발할 시간입니다. ${after.utterance}',
      cueKey: after.cueKey,
    );
