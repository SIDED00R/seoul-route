/// 경유지 체류 표기. "바로 통과" / "5분" / "1시간 30분".
String stayLabel(int minutes) {
  if (minutes <= 0) return '바로 통과';
  final h = minutes ~/ 60, m = minutes % 60;
  if (h == 0) return '$m분';
  return m == 0 ? '$h시간' : '$h시간 $m분';
}
