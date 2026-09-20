import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/guide/guide_store.dart';
import 'package:seoul_route/models/itinerary.dart';
import 'package:seoul_route/models/place.dart';
import 'package:seoul_route/models/plan_request.dart';

Map<String, dynamic> _legJson(String to, double toLat) => {
      'mode': 'WALK',
      'duration_sec': 300,
      'distance_m': 222,
      'from_name': '출발',
      'to_name': to,
      'from_lat': 37.5,
      'from_lon': 127.0,
      'to_lat': toLat,
      'to_lon': 127.0,
      'polyline': 'abc',
      'start': '2026-09-20T09:00:00+09:00',
      'end': '2026-09-20T09:05:00+09:00',
      'crossings': 2,
      'crossing_wait_sec': 76,
      'steps': [
        {'dir': 'DEPART', 'street': '테헤란로', 'distance_m': 120, 'lat': 37.5, 'lon': 127.0}
      ],
    };

Map<String, dynamic> _itineraryJson(List<Map<String, dynamic>> legs) => {
      'start': '2026-09-20T09:00:00+09:00',
      'end': '2026-09-20T09:05:00+09:00',
      'duration_sec': 300,
      'transfers': 0,
      'walk_distance_m': 222,
      'legs': legs,
    };

final _request = PlanRequest(
  origin: const Place(name: '출발', address: '서울 어딘가', lat: 37.5, lon: 127.0),
  destination: const Place(name: '도착지', address: '서울 어딘가', lat: 37.502, lon: 127.0),
  via: const [Place(name: '경유', address: '', lat: 37.501, lon: 127.001)],
  segmentModes: const [SegmentMode.walk, SegmentMode.bike],
);

GuideSnapshot _snap(DateTime startedAt, {int legIndex = 1}) {
  final legs = [_legJson('경유', 37.501), _legJson('도착지', 37.502)];
  return GuideSnapshot(
    tripId: 'T1',
    request: _request,
    itinerary: Itinerary.fromJson(_itineraryJson(legs)),
    legs: [for (final l in legs) Leg.fromJson(l)],
    legIndex: legIndex,
    shift: const Duration(seconds: 95),
    startedAt: startedAt,
  );
}

void main() {
  const store = GuideStore();
  final now = DateTime.parse('2026-09-20T10:00:00+09:00');

  setUp(() => SharedPreferences.setMockInitialValues(<String, Object>{}));

  test('남긴 안내를 그대로 되읽는다', () async {
    await store.save(_snap(now.subtract(const Duration(minutes: 20))));
    final got = await store.load(now: now);
    expect(got, isNotNull);
    expect(got!.tripId, 'T1');
    expect(got.legIndex, 1);
    expect(got.shift, const Duration(seconds: 95));
    expect(got.startedAt, now.subtract(const Duration(minutes: 20)));
    expect(got.request.destination.name, '도착지');
    expect(got.request.via.single.lat, 37.501);
    expect(got.request.segmentModes, [SegmentMode.walk, SegmentMode.bike]);
    // leg 는 원본 JSON 을 그대로 남기므로 폴리라인·횡단보도·단계까지 살아 있어야 한다.
    expect(got.legs.length, 2);
    expect(got.legs[1].toName, '도착지');
    expect(got.legs[1].polyline, 'abc');
    expect(got.legs[1].crossings, 2);
    expect(got.legs[1].steps.single.street, '테헤란로');
    expect(got.itinerary.legs.length, 2);
    expect(got.itinerary.durationSec, 300);
  });

  test('시작한 지 오래된 안내는 되살리지 않고 지운다', () async {
    await store.save(_snap(now.subtract(GuideStore.maxAge + const Duration(minutes: 1))));
    expect(await store.load(now: now), isNull);
    expect((await SharedPreferences.getInstance()).getString('active_guide'), isNull);
  });

  test('기한 안이면 살린다(경계)', () async {
    await store.save(_snap(now.subtract(GuideStore.maxAge)));
    expect(await store.load(now: now), isNotNull);
  });

  test('읽을 수 없는 값은 지워 다음 실행에서 또 걸리지 않게 한다', () async {
    SharedPreferences.setMockInitialValues(<String, Object>{'active_guide': '{"trip_id":'});
    expect(await store.load(now: now), isNull);
    expect((await SharedPreferences.getInstance()).getString('active_guide'), isNull);
  });

  test('남긴 값이 없으면 null', () async {
    expect(await store.load(now: now), isNull);
  });

  test('clear 는 남긴 안내를 지운다', () async {
    await store.save(_snap(now));
    await store.clear();
    expect(await store.load(now: now), isNull);
  });

  test('전 구간 any 면 segment_modes 가 빠지고, 되읽을 때 구간 수만큼 any 로 채운다', () async {
    final req = PlanRequest(
      origin: const Place(name: 'A', address: '', lat: 37.5, lon: 127.0),
      destination: const Place(name: 'B', address: '', lat: 37.502, lon: 127.0),
      via: const [Place(name: 'V', address: '', lat: 37.501, lon: 127.0)],
      segmentModes: const [SegmentMode.any, SegmentMode.any],
    );
    expect(req.toJson().containsKey('segment_modes'), isFalse);
    final back = PlanRequest.fromJson(jsonDecode(jsonEncode(req.toJson())) as Map<String, dynamic>);
    expect(back.segmentModes, [SegmentMode.any, SegmentMode.any]);
  });
}
