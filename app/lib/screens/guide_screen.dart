import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_activity_recognition/flutter_activity_recognition.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:geolocator/geolocator.dart';
import 'package:latlong2/latlong.dart';

import '../api/client.dart';
import '../guide/activity_classifier.dart';
import '../guide/background_location.dart';
import '../guide/end_confirm.dart';
import '../guide/leg_tracker.dart';
import '../guide/trace_uploader.dart';
import '../models/itinerary.dart';
import '../models/plan_request.dart';
import '../models/trace_sample.dart';
import '../util/leg_names.dart';
import '../util/polyline.dart';
import '../widgets/mode_icon.dart';

/// 안내 화면. 5초마다 위치를 받아 현재 구간을 넘기고 서버 trip 에 샘플을 올린다. 위치는 포그라운드 서비스(상단 알림)로
/// 받아 화면이 꺼지거나 다른 앱으로 넘어가도 이어진다.
/// "안내 종료" 를 누르면 남은 샘플을 보내고 trip 을 닫는다 — 서버가 그때 이번 안내의 걷기·자전거 속도를 내 프로파일에 반영한다.
class GuideScreen extends StatefulWidget {
  const GuideScreen({super.key, required this.api, required this.request, required this.itinerary});

  final ApiClient api;
  final PlanRequest request;
  final Itinerary itinerary;

  @override
  State<GuideScreen> createState() => _GuideScreenState();
}

class _GuideScreenState extends State<GuideScreen> {
  // 5초 간격: 서버 speed 패키지의 연속 쌍 간격(2~30초) 안에서 배터리와 표본 수의 절충. 서버 상수를 바꾸면 같이 본다.
  static const sampleInterval = Duration(seconds: 5);

  late final LegTracker _tracker = LegTracker(widget.itinerary.legs);
  final ActivityClassifier _activity = ActivityClassifier();
  final MapController _map = MapController();
  TraceUploader? _uploader;
  StreamSubscription<Position>? _positions;
  StreamSubscription<Activity>? _activities;
  bool _activityOn = false; // 활동 인식 권한을 받아 스트림을 켰는지. 아니면 샘플에 activity 를 싣지 않는다
  LatLng? _here;
  double? _accuracyM;
  int _samples = 0;
  String _status = '준비 중…';
  bool _ending = false;

  @override
  void initState() {
    super.initState();
    _start();
  }

  Future<void> _start() async {
    var perm = await Geolocator.checkPermission();
    if (perm == LocationPermission.denied) {
      perm = await Geolocator.requestPermission();
    }
    if (!mounted) return;
    if (perm == LocationPermission.denied || perm == LocationPermission.deniedForever) {
      setState(() => _status = '위치 권한이 없어 안내를 시작할 수 없습니다. 설정에서 허용한 뒤 다시 시작하세요.');
      return;
    }
    if (!await Geolocator.isLocationServiceEnabled()) {
      if (mounted) setState(() => _status = '기기 위치 서비스가 꺼져 있습니다.');
      return;
    }
    // 거부해도 안내는 계속한다(알림창에 안내 알림만 안 뜬다).
    await requestNotificationPermission();
    if (!mounted) return;
    // 위치 포그라운드 서비스는 앱이 백그라운드로 간 뒤에는 시작되지 않는다(Android 12+, ForegroundServiceStartNotAllowedException).
    // 스트림은 trip 발급을 기다리기 전에 연다. 발급 전 샘플은 서버에 올리지 않는다.
    _positions = Geolocator.getPositionStream(locationSettings: guideLocationSettings(sampleInterval)).listen(
        _onPosition, onError: (e) {
      if (mounted) setState(() => _status = '위치 오류: $e');
    });
    try {
      final tripId = await widget.api.startTrip();
      if (!mounted) {
        // trip 발급을 기다리는 동안 화면이 닫혔다면 서버에 열린 기록을 남기지 않는다.
        try {
          await widget.api.endTrip(tripId);
        } catch (_) {
          // 화면이 이미 사라졌으므로 정리는 최선 노력으로 끝낸다.
        }
        return;
      }
      _uploader = TraceUploader(api: widget.api, tripId: tripId)..start();
    } catch (e) {
      await _positions?.cancel();
      _positions = null;
      if (mounted) setState(() => _status = 'trip 발급 실패: $e');
      return;
    }
    setState(() => _status = '안내 중');
    await _startActivity();
  }

  /// 활동 인식은 있으면 좋은 것이라 권한이 없어도 안내는 계속한다(샘플의 activity 만 빠진다).
  Future<void> _startActivity() async {
    final ar = FlutterActivityRecognition.instance;
    try {
      var perm = await ar.checkPermission();
      if (perm == ActivityPermission.DENIED) {
        perm = await ar.requestPermission();
      }
      if (!mounted || perm != ActivityPermission.GRANTED) return;
      // 판정이 바뀔 때만 오는 스트림. 확정은 위치 샘플 시점(_onPosition 의 settle)에 한다.
      _activities = ar.activityStream.listen((a) {
        if (mounted) {
          _activity.observe(a.type.name, a.confidence.name, DateTime.now());
        }
      }, onError: (_) {});
      setState(() => _activityOn = true);
    } catch (_) {
      // 플러그인·Play 서비스 없음 등: 활동 없이 진행
    }
  }

  void _onPosition(Position p) {
    if (!mounted || _ending) return;
    _activity.settle(DateTime.now());
    final changed = _tracker.update(p.latitude, p.longitude);
    _uploader?.add(TraceSample(
      ts: p.timestamp,
      lat: p.latitude,
      lon: p.longitude,
      accuracyM: p.accuracy,
      mode: _tracker.mode,
      activity: _activityOn ? _activity.current : null,
    ));
    setState(() {
      _here = LatLng(p.latitude, p.longitude);
      _accuracyM = p.accuracy;
      _samples++;
    });
    if (changed) _fitCurrentLeg();
  }

  void _fitCurrentLeg() {
    final leg = _tracker.current;
    final pts = leg.polyline.isEmpty
        ? [LatLng(leg.fromLat, leg.fromLon), LatLng(leg.toLat, leg.toLon)]
        : decodePolyline(leg.polyline);
    _map.fitCamera(CameraFit.bounds(bounds: LatLngBounds.fromPoints(pts), padding: const EdgeInsets.all(48)));
  }

  Future<void> _end() async {
    final up = _uploader;
    setState(() {
      _ending = true;
      _status = '샘플 전송 중…';
    });
    await _positions?.cancel();
    _positions = null;
    if (up == null) {
      if (mounted) Navigator.pop(context);
      return;
    }
    await up.flush();
    if (!mounted) return;
    if (up.pending > 0) {
      setState(() {
        _ending = false;
        _status = '샘플 ${up.pending}개 전송 실패(${up.lastError}). 다시 종료를 누르면 재전송합니다.';
      });
      return;
    }
    try {
      final res = await widget.api.endTrip(up.tripId);
      if (!mounted) return;
      await showDialog<void>(
        context: context,
        builder: (_) => AlertDialog(
          title: const Text('안내 종료'),
          content: Text(_summary(res)),
          actions: [TextButton(onPressed: () => Navigator.pop(context), child: const Text('확인'))],
        ),
      );
      if (mounted) Navigator.pop(context);
    } on ApiException catch (e) {
      if (!mounted) return;
      // 409 = 서버가 이미 이 trip 을 닫고 학습까지 반영한 상태(앞선 종료 요청의 응답만 잃은 경우). 완료로 본다.
      if (e.status == 409) {
        setState(() => _status = '이미 종료된 안내입니다.');
        Navigator.pop(context);
        return;
      }
      _endFailed(e);
    } catch (e) {
      if (mounted) _endFailed(e);
    }
  }

  /// 뒤로가기: "종료" 를 고르면 안내 종료 버튼과 같은 흐름(샘플 전송·속도 반영)을 탄다.
  Future<void> _onBack() async {
    if (_ending) return;
    if (await confirmEndGuide(context) && mounted && !_ending) await _end();
  }

  void _endFailed(Object e) {
    setState(() {
      _ending = false;
      _status = '종료 실패: $e. 다시 종료를 누르세요.';
    });
  }

  /// 종료 응답 요약: 이번 안내의 수단별 속도(채택 여부)와 갱신된 내 속도.
  String _summary(Map<String, dynamic> res) {
    final trip = (res['trip'] as Map<String, dynamic>?) ?? const {};
    final profile = (res['profile'] as Map<String, dynamic>?) ?? const {};
    String line(String mode, String name) {
      final t = (trip[mode] as Map<String, dynamic>?) ?? const {};
      final v = t['speed_mps'];
      final tripText = v == null
          ? '표본 없음'
          : '${(v as num).toStringAsFixed(2)} m/s (${t['pairs']}쌍, ${t['used'] == true ? '반영' : '표본 부족·미반영'})';
      final mm = (t['mismatch'] as num?)?.toInt() ?? 0;
      final mmText = mm > 0 ? ' · 활동 불일치 제외 $mm쌍' : '';
      final pj = profile[mode];
      final p = pj == null ? '' : ' · 내 속도 ${SpeedProfile.fromJson(pj as Map<String, dynamic>).label}';
      return '$name: $tripText$mmText$p';
    }

    return '샘플 ${res['samples']}개\n${line('walk', '걷기')}\n${line('bicycle', '자전거')}';
  }

  @override
  void dispose() {
    _positions?.cancel();
    _activities?.cancel();
    _uploader?.dispose();
    _map.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final legs = widget.itinerary.legs;
    final polylines = <Polyline>[];
    final all = <LatLng>[];
    for (var i = 0; i < legs.length; i++) {
      final leg = legs[i];
      final pts = leg.polyline.isEmpty
          ? [LatLng(leg.fromLat, leg.fromLon), LatLng(leg.toLat, leg.toLon)]
          : decodePolyline(leg.polyline);
      all.addAll(pts);
      final isCurrent = i == _tracker.index;
      polylines.add(Polyline(
        points: pts,
        color: isCurrent ? modeColor(leg) : modeColor(leg).withValues(alpha: 0.35),
        strokeWidth: isCurrent ? 8 : 4,
        pattern: leg.mode == 'WALK' ? const StrokePattern.dotted() : const StrokePattern.solid(),
      ));
    }
    final here = _here;
    final leg = _tracker.current;
    final remainM = here == null ? null : LegTracker.distanceM(here.latitude, here.longitude, leg.toLat, leg.toLon);
    final up = _uploader;
    final mismatch = _activityOn && ActivityClassifier.mismatch(_tracker.mode, _activity.current);
    // trip 을 발급받은 뒤에는 뒤로가기(시스템 제스처·앱바 화살표)로 바로 나가지 않고 종료할지 묻는다.
    // 종료 흐름 안의 Navigator.pop 은 canPop 과 무관하게 화면을 닫는다.
    return PopScope(
      canPop: _uploader == null,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) _onBack();
      },
      child: Scaffold(
      appBar: AppBar(title: Text('안내 · 구간 ${_tracker.index + 1}/${legs.length}')),
      body: Column(
        children: [
          Expanded(
            child: FlutterMap(
              mapController: _map,
              options: MapOptions(
                initialCameraFit:
                    CameraFit.bounds(bounds: LatLngBounds.fromPoints(all), padding: const EdgeInsets.all(32)),
              ),
              children: [
                TileLayer(
                  urlTemplate: widget.api.tileUrlTemplate,
                  tileProvider: NetworkTileProvider(headers: widget.api.tileHeaders),
                  userAgentPackageName: 'kr.seoulroute.seoul_route',
                  retinaMode: true,
                ),
                PolylineLayer(polylines: polylines),
                MarkerLayer(markers: [
                  Marker(
                    point: LatLng(leg.toLat, leg.toLon),
                    width: 32,
                    height: 32,
                    child: const Icon(Icons.flag, color: Colors.orange, size: 28),
                  ),
                  if (here != null)
                    Marker(
                      point: here,
                      width: 28,
                      height: 28,
                      child: const Icon(Icons.my_location, color: Colors.blue, size: 26),
                    ),
                ]),
                const RichAttributionWidget(
                  attributions: [TextSourceAttribution('© VWorld(국토교통부) · OSM · 서울시 따릉이')],
                ),
              ],
            ),
          ),
          // 실기기(S23 울트라)의 시스템 내비게이션 바가 하단 패널을 덮으므로 아래 인셋만큼 띄운다(에뮬레이터에는 바가 없다).
          SafeArea(
            top: false,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: Icon(modeIcon(leg), color: modeColor(leg)),
                    title: Text('${leg.label} · ${(leg.durationSec / 60).round()}분'),
                    subtitle: Text('${legEndpointName(widget.request, leg.fromName, leg.fromLat, leg.fromLon)} → '
                        '${legEndpointName(widget.request, leg.toName, leg.toLat, leg.toLon)}'
                        '${remainM == null ? '' : ' · 끝까지 ${remainM.round()}m'}'),
                  ),
                  Row(
                    children: [
                      OutlinedButton(
                        onPressed: _tracker.index > 0 && !_ending ? () => setState(_tracker.prev) : null,
                        child: const Text('이전 구간'),
                      ),
                      const SizedBox(width: 8),
                      OutlinedButton(
                        onPressed: !_tracker.isLast && !_ending ? () => setState(_tracker.next) : null,
                        child: const Text('다음 구간'),
                      ),
                      const Spacer(),
                      FilledButton.icon(
                        onPressed: _ending ? null : _end,
                        icon: const Icon(Icons.stop),
                        label: const Text('안내 종료'),
                      ),
                    ],
                  ),
                  const SizedBox(height: 4),
                  if (mismatch)
                    Text(
                      '감지된 활동(${ActivityClassifier.label(_activity.current)})이 이 구간과 다릅니다. '
                      '구간을 넘기거나 안내를 종료하세요 — 이 동안의 샘플은 속도 학습에서 뺍니다.',
                      style: TextStyle(color: Theme.of(context).colorScheme.error),
                    ),
                  Text(
                    '$_status · 샘플 $_samples'
                    '${_activityOn ? ' · 활동 ${ActivityClassifier.label(_activity.current)}' : ''}'
                    '${_accuracyM == null ? '' : ' · 정확도 ${_accuracyM!.round()}m'}'
                    '${up == null ? '' : ' · 업로드 ${up.uploaded} · 대기 ${up.pending}'
                        '${up.failures > 0 ? ' · 실패 ${up.failures}' : ''}'}',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
      ),
    );
  }
}
