import 'package:flutter/services.dart';

const _channel = MethodChannel('seoul_route/app_task');

/// 앱을 끝내지 않고 뒤로 보낸다(Android moveTaskToBack). 안내 중 뒤로가기로 액티비티가 끝나면 Dart 쪽이 통째로
/// 사라져 위치 스트림·알림창·안내 상태가 같이 끊긴다. 보냈으면 true, 채널이 없거나(테스트·다른 플랫폼)
/// 실패하면 false — 그때는 호출자가 아무것도 하지 않아 안내가 남는다.
Future<bool> moveAppToBack() async {
  try {
    return await _channel.invokeMethod<bool>('moveToBack') ?? false;
  } on MissingPluginException {
    return false;
  } on PlatformException {
    return false;
  }
}
