/// 길찾기 시각 조건: 지금 출발, 이 시각에 출발, 이 시각까지 도착.
enum PlanTimeKind {
  now('지금 출발'),
  depart('출발 시각'),
  arrive('도착 시각');

  const PlanTimeKind(this.label);
  final String label;
}

class PlanTime {
  const PlanTime(this.kind, this.at);

  static const now = PlanTime(PlanTimeKind.now, null);

  final PlanTimeKind kind;
  final DateTime? at; // now 면 null

  DateTime? get depart => kind == PlanTimeKind.depart ? at : null;
  DateTime? get arrive => kind == PlanTimeKind.arrive ? at : null;

  /// 경로 찾기 버튼 문구에 쓰는 시각: "09:05 출발", "내일 09:05 도착", "10월 5일(월) 09:05 도착". now 면 "지금 출발".
  String label(DateTime today) {
    final t = at;
    if (kind == PlanTimeKind.now || t == null) return PlanTimeKind.now.label;
    final verb = kind == PlanTimeKind.depart ? '출발' : '도착';
    return '${dayLabel(t, today)}${hhmm(t)} $verb';
  }
}

/// 오늘이면 "", 내일이면 "내일 ", 그 밖이면 "10월 5일(월) ".
String dayLabel(DateTime t, DateTime today) {
  final d = DateTime(
    t.year,
    t.month,
    t.day,
  ).difference(DateTime(today.year, today.month, today.day)).inDays;
  if (d == 0) return '';
  if (d == 1) return '내일 ';
  const week = ['월', '화', '수', '목', '금', '토', '일'];
  return '${t.month}월 ${t.day}일(${week[t.weekday - 1]}) ';
}

String hhmm(DateTime t) =>
    '${t.hour.toString().padLeft(2, '0')}:${t.minute.toString().padLeft(2, '0')}';
