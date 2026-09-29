import 'dart:async';

/// 아직 끝나지 않은 발화 수를 세어 오디오 포커스를 잡고 놓는다: 0 에서 1 이 되면 duck, 0 이 되면 release. 발화 끝을
/// 알리는 콜백이 오지 않아도 마지막 소식 뒤 [maxQuiet] 가 지나면 놓는다.
class SpeechFocus {
  SpeechFocus({required this.duck, required this.release, this.maxQuiet = const Duration(seconds: 20)});

  final Future<void> Function() duck;
  final Future<void> Function() release;
  final Duration maxQuiet;

  int _pending = 0;
  Timer? _quiet;

  int get pending => _pending;

  /// 발화를 하나 시작하기 직전에 부른다. duck 응답을 기다리지 않는다.
  void began() {
    _pending++;
    _arm();
    if (_pending == 1) unawaited(duck());
  }

  /// 발화 하나가 끝났을(완료·취소·오류) 때 부른다.
  void ended() {
    if (_pending == 0) return;
    _pending--;
    if (_pending == 0) {
      _clear();
    } else {
      _arm();
    }
  }

  /// 남은 발화를 모두 버린다(읽기 중지·안내 종료).
  void reset() {
    if (_pending == 0) return;
    _pending = 0;
    _clear();
  }

  void _arm() {
    _quiet?.cancel();
    _quiet = Timer(maxQuiet, reset);
  }

  void _clear() {
    _quiet?.cancel();
    _quiet = null;
    release();
  }
}
