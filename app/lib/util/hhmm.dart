/// "HH:MM". 안내 카드·알림창·현재 경로 탭이 같은 형식을 쓴다.
String hhmm(DateTime t) => '${t.hour.toString().padLeft(2, '0')}:${t.minute.toString().padLeft(2, '0')}';
