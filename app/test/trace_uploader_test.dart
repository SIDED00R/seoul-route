import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/guide/trace_uploader.dart';
import 'package:seoul_route/models/trace_sample.dart';

/// 처음 failFirst 번은 실패하는 서버. 받은 배치를 기록한다.
class FlakyApi extends ApiClient {
  FlakyApi({this.failFirst = 0, this.failStatus = 500}) : super(baseUrl: 'http://x', token: 't');

  int failFirst;
  final int failStatus;
  final List<List<TraceSample>> batches = [];

  @override
  Future<void> uploadTraces(String tripId, List<TraceSample> samples) async {
    await Future.delayed(const Duration(milliseconds: 10));
    if (failFirst > 0) {
      failFirst--;
      throw ApiException(failStatus, failStatus == 409 ? '종료된 trip' : '저장 실패');
    }
    batches.add(List.of(samples));
  }
}

TraceSample sample(int i) =>
    TraceSample(ts: DateTime.utc(2026, 9, 13, 9, 0, i * 5), lat: 37.55, lon: 126.97, accuracyM: 5, mode: 'walk');

void main() {
  test('배치 크기에 닿으면 보내고, 실패분은 다음 배치와 합쳐 재전송한다', () async {
    final api = FlakyApi(failFirst: 1);
    final up = TraceUploader(api: api, tripId: 'T', batchSize: 3);
    for (var i = 0; i < 3; i++) {
      up.add(sample(i));
    }
    await Future.delayed(const Duration(milliseconds: 50)); // 첫 전송 실패
    expect(api.batches, isEmpty);
    expect(up.failures, 1);
    expect(up.pending, 3);
    up.add(sample(3)); // 실패분 3 + 새 1 = 4 ≥ 3 → 재전송
    await Future.delayed(const Duration(milliseconds: 50));
    expect(api.batches.length, 1);
    expect(api.batches.single.length, 4);
    expect(api.batches.single.first.ts, sample(0).ts); // 실패분이 앞에 온다
    expect(up.uploaded, 4);
    expect(up.pending, 0);
    up.dispose();
  });

  test('flush 는 남은 샘플을 보내고, 그래도 실패하면 pending 에 남긴다', () async {
    final api = FlakyApi(failFirst: 1);
    final up = TraceUploader(api: api, tripId: 'T', batchSize: 100);
    up.add(sample(0));
    up.add(sample(1));
    await up.flush();
    expect(up.pending, 2);
    expect(up.lastError, contains('500'));
    await up.flush(); // 두 번째는 성공
    expect(up.pending, 0);
    expect(api.batches.single.length, 2);
    expect(up.lastError, isNull);
  });

  test('서버 상한을 넘는 큐는 1000개씩 나눠 보내고 flush 는 다 빌 때까지 반복한다', () async {
    final api = FlakyApi();
    final up = TraceUploader(api: api, tripId: 'T', batchSize: 100000);
    for (var i = 0; i < 1500; i++) {
      up.add(sample(i));
    }
    await up.flush();
    expect(api.batches.map((b) => b.length).toList(), [1000, 500]);
    expect(api.batches.first.first.ts, sample(0).ts);
    expect(api.batches.last.first.ts, sample(1000).ts);
    expect(up.pending, 0);
    expect(up.uploaded, 1500);
  });

  test('서버가 trip 을 이미 닫았으면(409) 큐를 비운다(실패로 세지 않는다)', () async {
    final api = FlakyApi(failFirst: 1, failStatus: 409);
    final up = TraceUploader(api: api, tripId: 'T', batchSize: 100);
    up.add(sample(0));
    up.add(sample(1));
    await up.flush();
    expect(up.pending, 0);
    expect(up.failures, 0);
    expect(api.batches, isEmpty);
  });

  test('전송 중 들어온 샘플은 잃지 않는다', () async {
    final api = FlakyApi();
    final up = TraceUploader(api: api, tripId: 'T', batchSize: 2);
    up.add(sample(0));
    up.add(sample(1)); // 전송 시작(10ms)
    up.add(sample(2)); // 전송 중 도착
    await Future.delayed(const Duration(milliseconds: 50));
    expect(api.batches.length, 1);
    expect(up.pending, 1);
    await up.flush();
    expect(api.batches.length, 2);
    expect(api.batches.last.single.ts, sample(2).ts);
  });
}
