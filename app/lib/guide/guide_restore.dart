import 'package:geolocator/geolocator.dart';

import '../api/client.dart';
import 'active_guide.dart';
import 'guide_session.dart';
import 'guide_store.dart';

/// 앱이 켜질 때 디스크에 남은 안내를 되살린다. 남은 것이 없거나 이미 안내 중이면 아무 일도 하지 않는다.
/// 되살린 안내는 위치 스트림·trip 업로드를 다시 걸고 저장된 구간부터 이어간다.
///
/// 위치 권한·기기 위치가 준비되지 않았으면 세션을 만들기 전에 물러난다. 만들어 두면 위치 스트림도 업로더도 없는
/// 안내가 활성으로 남고(GuideScreen 은 이미 있는 세션에 start() 를 다시 부르지 않는다), 그 세션을 치우면
/// GuideSession.dispose 가 디스크에 남은 것까지 지운다. 남겨 두면 권한이 갖춰진 다음 실행에서 되살아난다.
/// 권한을 새로 묻지는 않는다 — 앱을 열자마자 권한 창을 띄우지 않는다.
Future<bool> restoreGuide(ApiClient api, {GuideStore store = const GuideStore()}) async {
  if (ActiveGuide.instance.current != null) return false;
  final snap = await store.load();
  if (snap == null) return false;
  final perm = await Geolocator.checkPermission();
  if (perm == LocationPermission.denied || perm == LocationPermission.deniedForever) return false;
  if (!await Geolocator.isLocationServiceEnabled()) return false;
  // 위 두 호출은 플랫폼을 다녀오므로 그동안 이벤트 루프가 돈다. 그 사이 새 안내가 걸렸으면 덮어쓰지 않는다.
  if (ActiveGuide.instance.current != null) return false;
  final s = GuideSession.resume(snap, api: api, store: store);
  ActiveGuide.instance.set(s);
  await s.start();
  return true;
}
