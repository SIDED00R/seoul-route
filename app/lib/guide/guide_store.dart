import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

import '../models/itinerary.dart';
import '../models/plan_request.dart';

/// 진행 중인 안내 한 건. 이어받는 데 필요한 것만 담는다 — 서버 trip, 요청·여정 원본, 지금 구간과 밀린 시간.
class GuideSnapshot {
  const GuideSnapshot({
    required this.tripId,
    required this.request,
    required this.itinerary,
    required this.legs,
    required this.legIndex,
    required this.shift,
    required this.startedAt,
  });

  final String tripId;
  final PlanRequest request;
  final Itinerary itinerary;

  /// 지금 따라가는 구간 목록. 경로 이탈 재탐색으로 갈아 낀 구간이 들어 있어 itinerary.legs 와 다를 수 있다.
  final List<Leg> legs;
  final int legIndex;
  final Duration shift;
  final DateTime startedAt;

  Map<String, dynamic> toJson() => {
        'trip_id': tripId,
        'request': request.toJson(),
        'itinerary': itinerary.raw,
        'legs': [for (final l in legs) l.raw],
        'leg_index': legIndex,
        'shift_sec': shift.inSeconds,
        'started_at': startedAt.toIso8601String(),
      };

  factory GuideSnapshot.fromJson(Map<String, dynamic> j) => GuideSnapshot(
        tripId: j['trip_id'] as String,
        request: PlanRequest.fromJson(j['request'] as Map<String, dynamic>),
        itinerary: Itinerary.fromJson(j['itinerary'] as Map<String, dynamic>),
        legs: [
          for (final l in (j['legs'] as List<dynamic>)) Leg.fromJson(l as Map<String, dynamic>)
        ],
        legIndex: (j['leg_index'] as num).toInt(),
        shift: Duration(seconds: (j['shift_sec'] as num).toInt()),
        startedAt: DateTime.parse(j['started_at'] as String),
      );
}

/// 진행 중인 안내를 디스크에 둔다. 앱이 죽어도(시스템 정리·강제 종료) 다시 켤 때 같은 안내를 이어받는다.
/// 한 번에 하나뿐이라 키도 하나다 — 새 안내를 저장하면 앞 것을 덮는다.
class GuideStore {
  const GuideStore();

  static const _key = 'active_guide';

  /// 시작한 지 이보다 오래된 안내는 되살리지 않는다. 서버가 조용한 trip 을 닫는 기준(backend speed.StaleAfter)과
  /// 값은 같지만 재는 시점이 다르다 — 서버는 마지막 표본부터, 여기는 시작 시각부터 센다. 그래서 이쪽 기한이
  /// 이르거나 같고(표본이 하나도 없으면 같다), 기한 안이면 서버가 그 trip 을 닫았을 수 없다.
  /// 대신 6시간 넘게 이어진 안내는 되살리지 않는다.
  static const maxAge = Duration(hours: 6);

  Future<void> save(GuideSnapshot s) async {
    try {
      final p = await SharedPreferences.getInstance();
      await p.setString(_key, jsonEncode(s.toJson()));
    } catch (_) {
      // 저장에 실패해도 안내는 이어간다(앱이 죽었을 때 이어받지 못할 뿐이다)
    }
  }

  /// 남아 있는 안내. 없거나 너무 오래됐거나 읽을 수 없으면 null 이고 그 값은 지운다.
  Future<GuideSnapshot?> load({DateTime? now}) async {
    final SharedPreferences p;
    try {
      p = await SharedPreferences.getInstance();
    } catch (_) {
      return null; // 플러그인 없음(테스트·다른 플랫폼)
    }
    final raw = p.getString(_key);
    if (raw == null) return null;
    GuideSnapshot s;
    try {
      s = GuideSnapshot.fromJson(jsonDecode(raw) as Map<String, dynamic>);
    } catch (_) {
      await p.remove(_key); // 형식이 바뀌었거나 깨진 값 — 다음 실행에서 또 걸리지 않게 지운다
      return null;
    }
    if ((now ?? DateTime.now()).difference(s.startedAt) > maxAge) {
      await p.remove(_key);
      return null;
    }
    return s;
  }

  Future<void> clear() async {
    try {
      await (await SharedPreferences.getInstance()).remove(_key);
    } catch (_) {
      // 지우지 못해도 다음 load 가 나이로 걸러 낸다
    }
  }
}
