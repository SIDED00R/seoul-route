import 'package:flutter/foundation.dart';

import 'guide_session.dart';

/// 지금 진행 중인 안내 하나. 안내 화면을 닫아도 여기에 남아 있어 "현재 경로" 탭이 보여 주고, 다시 열 수 있다.
/// 안내는 한 번에 하나만 둔다 — 위치 스트림과 알림창이 하나뿐이다.
class ActiveGuide {
  ActiveGuide._();

  static final ActiveGuide instance = ActiveGuide._();

  /// 진행 중인 안내. 없으면 null. 화면은 이 값을 듣는다.
  final ValueNotifier<GuideSession?> session = ValueNotifier<GuideSession?>(null);

  GuideSession? get current => session.value;

  /// 새 안내를 건다. 앞선 안내가 남아 있으면 놓고(서버 trip 은 닫지 않는다) 바꾼다 —
  /// 화면이 안내를 시작하기 전에 진행 중인 안내가 있는지 먼저 보므로 정상 흐름에서는 일어나지 않는다.
  void set(GuideSession s) {
    final old = session.value;
    session.value = s;
    old?.dispose();
  }

  /// 안내를 치운다. 세션 자체는 end() 로 이미 닫혀 있어야 한다.
  void clear() {
    final old = session.value;
    session.value = null;
    old?.dispose();
  }
}
