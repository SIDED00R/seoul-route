import 'dart:async';

import '../api/client.dart';
import '../models/trace_sample.dart';

/// 위치 샘플을 모아 배치로 올린다. 실패한 배치는 버리지 않고 다음 배치와 합쳐 다시 보낸다(서버가 (trip, ts) 중복을 무시하므로
/// 같은 샘플이 두 번 가도 된다). flush() 는 남은 것을 전부 보내고 끝난다(종료 직전에 부른다).
class TraceUploader {
  TraceUploader({
    required this.api,
    required this.tripId,
    this.batchSize = 20,
    this.interval = const Duration(seconds: 30),
    this.minGap = const Duration(seconds: 5),
  });

  final ApiClient api;
  final String tripId;
  final int batchSize; // 5초 샘플 20개 = 100초
  final Duration interval;
  // 샘플 사이 최소 간격. 화면은 더 자주 위치를 받지만 서버에 올리는 표본은 5초 간격을 지킨다 —
  // 서버 speed 패키지의 연속 쌍 간격(2~30초)과 표본 수 기준(MinPairs 12 = 5초 1분치)이 그 간격을 전제한다.
  final Duration minGap;

  final List<TraceSample> _pending = [];
  Timer? _timer;
  bool _sending = false;
  int uploaded = 0;
  int failures = 0;
  String? lastError;

  int get pending => _pending.length;

  void start() {
    _timer ??= Timer.periodic(interval, (_) => _send());
  }

  DateTime? _lastTs;

  void add(TraceSample s) {
    final last = _lastTs;
    if (last != null && s.ts.difference(last) < minGap) return;
    _lastTs = s.ts;
    _pending.add(s);
    if (_pending.length >= batchSize) _send();
  }

  // 한 요청 상한. 서버 httpapi.MaxTraceBatch 와 같은 값 — 넘기면 400 이라 오래 끊겼던 큐가 영영 못 나간다.
  static const maxPerRequest = 1000;

  Future<void> _send() async {
    if (_sending || _pending.isEmpty) return;
    _sending = true;
    // 전송 중 들어온 샘플은 _pending 뒤에 남는다. 보낼 묶음(앞에서 최대 maxPerRequest)만 떼어 두고 실패하면 앞에 되돌린다.
    final n = _pending.length < maxPerRequest ? _pending.length : maxPerRequest;
    final batch = _pending.sublist(0, n);
    _pending.removeRange(0, n);
    try {
      await api.uploadTraces(tripId, batch);
      uploaded += batch.length;
      lastError = null;
    } on ApiException catch (e) {
      // 409 = 서버가 이 trip 을 이미 닫았다. 더 받을 수 없으니 큐를 비워 호출자가 종료 요청으로 넘어가게 한다.
      if (e.status == 409) {
        _pending.clear();
      } else {
        failures++;
        lastError = e.toString();
        _pending.insertAll(0, batch);
      }
    } catch (e) {
      failures++;
      lastError = e.toString();
      _pending.insertAll(0, batch);
    } finally {
      _sending = false;
    }
  }

  /// 남은 샘플을 줄어드는 동안 계속 보낸다(요청당 maxPerRequest). 실패하면 pending 에 남긴 채 돌아온다(호출자가 표시).
  Future<void> flush() async {
    _timer?.cancel();
    _timer = null;
    while (_sending) {
      await Future.delayed(const Duration(milliseconds: 100));
    }
    while (_pending.isNotEmpty) {
      final before = _pending.length;
      await _send();
      if (_pending.length >= before) return;
    }
  }

  void dispose() {
    _timer?.cancel();
    _timer = null;
  }
}
