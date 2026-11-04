import 'package:flutter_test/flutter_test.dart';

import 'package:seoul_route/guide/geo.dart' as geo;
import 'package:seoul_route/guide/transit_deviation.dart';

// 합성 좌표. 위도 37.5 에서 경도 0.001도 ≈ 88m, 위도 0.001도 ≈ 111m.
const _boardLat = 37.5, _boardLon = 127.0;
const _alightLat = 37.5, _alightLon = 127.05; // 탑승역에서 동쪽으로 약 4.4km
const _step = 0.000085; // 경도 약 7.5m — 6초마다면 1.25m/s 걸음

final _t0 = DateTime(2026, 11, 4, 14);

class _Feed {
  _Feed() {
    detector.start(boardLat: _boardLat, boardLon: _boardLon, at: _t0);
  }

  final detector = TransitDeviationDetector();

  /// pathM 을 주지 않으면 계획 경로 위(0m)로 본다.
  TransitDeviation at(int sec, double lat, double lon,
          {double acc = 5, double pathM = 0, bool riding = false, bool watchWalk = true}) =>
      detector.update(
        lat: lat,
        lon: lon,
        accuracyM: acc,
        at: _t0.add(Duration(seconds: sec)),
        pathM: pathM,
        boardM: geo.distanceM(lat, lon, _boardLat, _boardLon),
        alightM: geo.distanceM(lat, lon, _alightLat, _alightLon),
        riding: riding,
        watchWalk: watchWalk,
      );

  /// start 초부터 6초마다 동쪽으로 걷는 표본 n 개. 각 결과를 돌려준다.
  List<TransitDeviation> walk(int start, int n, {double lon0 = 127.02, double acc = 5, bool riding = false}) => [
        for (var i = 0; i < n; i++) at(start + i * 6, 37.5, lon0 + i * _step, acc: acc, riding: riding),
      ];
}

void main() {
  group('내려서 걷기', () {
    test('두 역 모두에서 400m 밖을 GNSS 표본으로 걷는 속도로 60초·100m 넘게 가면 판정한다', () {
      // 지하철 바로 위 큰길을 걸어도(경로선 위) 판정한다. 100m 를 넘는 것은 15번째 표본(84초, 105m).
      final got = _Feed().walk(0, 15);
      expect(got.sublist(0, 14), everyElement(TransitDeviation.none));
      expect(got[14], TransitDeviation.walkedAway);
    });

    test('지하 정차 중 와이파이 표본(정확도 10m 넘음)은 120m 흔들려도 창에 들지 않는다', () {
      final f = _Feed();
      for (var i = 0; i < 40; i++) {
        // 4분 동안 정차, 6초마다 동서로 120m 안에서 흔들린다
        final d = f.at(i * 6, 37.5, 127.02 + (i % 4) * 0.00045, acc: 12);
        expect(d, TransitDeviation.none, reason: '$i');
      }
    });

    test('고가 역에 1분 서 있다 출발하는 열차(GNSS 표본)는 빠른 한 걸음이 창을 지운다', () {
      final f = _Feed();
      // 정차 60초 뒤 6초마다 5·10·20·35·50m 로 빨라진다. 시작점에서 걷는 속도 이하라 걸음 검사가 없으면 90초에 판정된다.
      const moved = [0, 5, 15, 35, 70, 120];
      final got = [
        for (var i = 0; i <= 10; i++) f.at(i * 6, 37.5, 127.02),
        for (var j = 1; j < moved.length; j++) f.at(60 + j * 6, 37.5, 127.02 + moved[j] / 88220),
      ];
      expect(got, everyElement(TransitDeviation.none));
    });

    test('위치가 2초마다 와도 고가 역에 1분 서 있다 0.5m/s² 로 출발하는 열차(정확도 8m)는 판정하지 않는다', () {
      final f = _Feed();
      final got = [
        for (var i = 0; i <= 30; i++) f.at(i * 2, 37.5, 127.02, acc: 8), // 정차 60초
        for (var tau = 2; tau <= 40; tau += 2) f.at(60 + tau, 37.5, 127.02 + 0.25 * tau * tau / 88220, acc: 8),
      ];
      expect(got, everyElement(TransitDeviation.none));
    });

    test('위치가 2초마다 와도 걷는 사람은 판정한다', () {
      final f = _Feed();
      // 2초마다 동쪽으로 2.5m(1.25m/s)
      final got = [for (var i = 0; i <= 60; i++) f.at(i * 2, 37.5, 127.02 + i * 2.5 / 88220)];
      expect(got, contains(TransitDeviation.walkedAway));
    });

    test('역 사이를 달리는 열차(역마다 1km)는 판정하지 않는다', () {
      final f = _Feed();
      for (var i = 0; i < 6; i++) {
        for (var j = 0; j < 8; j++) {
          expect(f.at(i * 90 + j * 6, 37.5, 127.01 + i * 0.0114, acc: 15), TransitDeviation.none);
        }
      }
    });

    test('탑승역이나 계획 하차역 400m 안의 걸음(출입구↔승강장)은 판정하지 않는다', () {
      expect(_Feed().walk(0, 30, lon0: 127.0), everyElement(TransitDeviation.none));
      expect(_Feed().walk(0, 30, lon0: 127.046), everyElement(TransitDeviation.none));
    });

    test('탈것 안이면 판정하지 않고, 정확도 8m 넘는 표본으로는 창을 열지 않는다', () {
      expect(_Feed().walk(0, 30, riding: true), everyElement(TransitDeviation.none));
      expect(_Feed().walk(0, 30, acc: 12), everyElement(TransitDeviation.none));
    });

    test('내려서 걷는 구간(watchWalk 거짓)에서는 보지 않는다', () {
      final f = _Feed();
      for (var i = 0; i < 30; i++) {
        expect(f.at(i * 6, 37.5, 127.02 + i * _step, watchWalk: false), TransitDeviation.none);
      }
    });
  });

  group('다른 열차', () {
    test('계획 경로 밖 400m 를 열차 속도로 온 표본이 두 번 이어지면 판정한다', () {
      final f = _Feed();
      expect(f.at(60, 37.5, 127.01, acc: 15), TransitDeviation.none); // 계획 경로 위
      // 2분 뒤 북동쪽 1.2km 역, 경로에서 600m. 같은 역에서 6초 뒤 한 번 더 찍히면 판정한다.
      expect(f.at(180, 37.507, 127.02, acc: 15, pathM: 600), TransitDeviation.none);
      expect(f.at(186, 37.507, 127.02, acc: 15, pathM: 600), TransitDeviation.otherTrain);
    });

    test('걸어서 계획 경로에서 멀어지면 다른 열차로 보지 않는다', () {
      final f = _Feed();
      expect(f.at(0, 37.5, 127.02, watchWalk: false), TransitDeviation.none);
      // 6초마다 북쪽으로 약 7.8m(1.3m/s). 400m 를 넘긴 뒤에도 다른 열차가 아니다.
      for (var i = 1; i <= 80; i++) {
        final pathM = i * 7.8;
        final d = f.at(i * 6, 37.5 + i * 0.00007, 127.02, pathM: pathM, watchWalk: false);
        expect(d, TransitDeviation.none, reason: '$pathM m');
      }
    });

    test('사이에 계획 경로 위 표본(지난 역 자리 튐)이 끼면 처음부터 다시 센다', () {
      final f = _Feed();
      expect(f.at(60, 37.5, 127.01, acc: 15), TransitDeviation.none);
      expect(f.at(180, 37.507, 127.02, acc: 15, pathM: 600), TransitDeviation.none);
      expect(f.at(186, 37.5, 127.01, acc: 20), TransitDeviation.none);
      expect(f.at(300, 37.507, 127.03, acc: 15, pathM: 600), TransitDeviation.none);
    });

    test('탑승역 없이 시작했으면 계획 경로 위 표본이 잡힐 때까지 세지 않는다', () {
      final f = TransitDeviationDetector()..start(at: _t0);
      TransitDeviation off(int sec, double lon, {double pathM = 600}) => f.update(
            lat: 37.507,
            lon: lon,
            accuracyM: 20,
            at: _t0.add(Duration(seconds: sec)),
            pathM: pathM,
            boardM: 1950,
            alightM: 2700,
            riding: false,
            watchWalk: false,
          );
      expect(off(2, 127.02), TransitDeviation.none);
      expect(off(4, 127.02), TransitDeviation.none);
      expect(off(60, 127.0, pathM: 100), TransitDeviation.none); // 계획 경로 위 — 여기서 기준점이 생긴다
      expect(off(180, 127.02), TransitDeviation.none);
      expect(off(186, 127.02), TransitDeviation.otherTrain);
    });

    test('계획 경로 위 표본이 없으면 구간에 들어온 때의 탑승역을 기준으로 잰다', () {
      final f = _Feed();
      // 들어온 지 3분 뒤 탑승역에서 1.9km — 열차 속도
      expect(f.at(180, 37.507, 127.02, acc: 20, pathM: 600), TransitDeviation.none);
      expect(f.at(186, 37.507, 127.02, acc: 20, pathM: 600), TransitDeviation.otherTrain);
    });
  });
}
