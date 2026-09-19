import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';

import '../api/client.dart';
import '../guide/active_guide.dart';
import '../guide/activity_classifier.dart';
import '../guide/guide_session.dart';
import '../guide/leg_tracker.dart';
import '../guide/voice_guide.dart';
import '../models/itinerary.dart';
import '../models/plan_request.dart';
import '../models/trace_sample.dart';
import '../util/leg_names.dart';
import '../widgets/guide_card.dart';
import '../widgets/mode_icon.dart';

/// 안내 화면. 진행 중인 안내(GuideSession)를 지도·카드·버튼으로 보여 준다. 안내 자체는 이 화면이 아니라 세션이
/// 들고 있어서 뒤로가기로 이 화면을 닫아도 위치 기록·음성·알림창은 이어진다 — "안내 종료" 를 눌러야 끝난다.
class GuideScreen extends StatefulWidget {
  const GuideScreen({
    super.key,
    required this.api,
    required this.request,
    required this.itinerary,
    this.speak,
    this.clock,
  });

  /// 이미 진행 중인 안내를 다시 연다("현재 경로" 탭·홈에서).
  GuideScreen.resume(GuideSession s, {super.key})
      : api = s.api,
        request = s.request,
        itinerary = s.itinerary,
        speak = s.speak,
        clock = null;

  final ApiClient api;
  final PlanRequest request;
  final Itinerary itinerary;

  /// 음성 안내 발화. 기본은 폰 내장 음성(TtsSpeaker)이고 테스트에서 바꿔 끼운다.
  final Speak? speak;

  /// 현재 시각. 시간표로 구간을 넘기는 판정에 쓰며 테스트에서 바꿔 끼운다.
  final DateTime Function()? clock;

  @override
  State<GuideScreen> createState() => _GuideScreenState();
}

class _GuideScreenState extends State<GuideScreen> {
  /// 따라가기 모드에서 지도를 이 배율 밑으로는 줄이지 않는다(도보 안내에서 모퉁이가 보이는 정도).
  static const followZoom = 17.0;

  final MapController _map = MapController();
  late final GuideSession _session;
  bool _follow = true; // 지도가 내 위치를 따라간다. 손으로 지도를 옮기면 꺼진다
  bool _mapReady = false;
  LatLng? _lastMoved;

  @override
  void initState() {
    super.initState();
    final running = ActiveGuide.instance.current;
    // 같은 여정으로 이미 안내 중이면 그 안내를 이어서 보여 준다(다시 시작하지 않는다).
    if (running != null && !running.ended && identical(running.itinerary, widget.itinerary)) {
      _session = running;
    } else {
      _session = GuideSession(
        api: widget.api,
        request: widget.request,
        itinerary: widget.itinerary,
        speak: widget.speak,
        clock: widget.clock,
      );
      ActiveGuide.instance.set(_session);
      _session.start();
    }
    _session.addListener(_onSession);
  }

  void _onSession() {
    if (!mounted) return;
    setState(() {});
    final here = _session.here;
    if (_follow && _mapReady && here != null && here != _lastMoved) {
      _lastMoved = here;
      _map.move(here, _map.camera.zoom < followZoom ? followZoom : _map.camera.zoom);
    }
  }

  Future<void> _end() async {
    final res = await _session.end();
    if (!mounted || res == null) return;
    if (res.isNotEmpty) {
      await showDialog<void>(
        context: context,
        builder: (_) => AlertDialog(
          title: const Text('안내 종료'),
          content: Text(_summary(res)),
          actions: [TextButton(onPressed: () => Navigator.pop(context), child: const Text('확인'))],
        ),
      );
    }
    ActiveGuide.instance.clear();
    if (mounted) Navigator.pop(context);
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
    // 세션은 여기서 끝내지 않는다 — 화면을 닫아도 안내는 이어진다.
    _session.removeListener(_onSession);
    _map.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = _session;
    final tracker = s.tracker;
    final legs = widget.itinerary.legs;
    final polylines = <Polyline>[];
    final all = <LatLng>[];
    for (var i = 0; i < legs.length; i++) {
      final leg = legs[i];
      final pts = tracker.points[i];
      all.addAll(pts);
      final isCurrent = i == tracker.index;
      polylines.add(Polyline(
        points: pts,
        color: isCurrent ? modeColor(leg) : modeColor(leg).withValues(alpha: 0.35),
        strokeWidth: isCurrent ? 8 : 4,
        pattern: leg.mode == 'WALK' ? const StrokePattern.dotted() : const StrokePattern.solid(),
      ));
    }
    final here = s.here;
    final leg = tracker.current;
    final remainM = here == null ? null : LegTracker.distanceM(here.latitude, here.longitude, leg.toLat, leg.toLon);
    final up = s.uploader;
    return Scaffold(
      appBar: AppBar(title: Text('안내 · 구간 ${tracker.index + 1}/${legs.length}')),
      body: Column(
        children: [
          GuideCard(
            icon: modeIcon(leg),
            color: modeColor(leg),
            eta: s.eta,
            remainMin: s.remainMin,
            now: s.instr.now,
            next: s.instr.next,
          ),
          Expanded(
            child: Stack(children: [
              FlutterMap(
                mapController: _map,
                options: MapOptions(
                  initialCameraFit:
                      CameraFit.bounds(bounds: LatLngBounds.fromPoints(all), padding: const EdgeInsets.all(32)),
                  onMapReady: () => _mapReady = true,
                  // 손으로 지도를 옮기면 따라가기를 멈춘다. 다시 켜는 버튼은 지도 오른쪽 아래에 나온다.
                  onPositionChanged: (_, hasGesture) {
                    if (hasGesture && _follow) setState(() => _follow = false);
                  },
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
              if (!_follow && here != null)
                Positioned(
                  right: 12,
                  bottom: 12,
                  child: FloatingActionButton.small(
                    heroTag: 'recenter',
                    tooltip: '내 위치로',
                    onPressed: () {
                      setState(() => _follow = true);
                      if (_mapReady) _map.move(here, followZoom);
                    },
                    child: const Icon(Icons.my_location),
                  ),
                ),
            ]),
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
                        onPressed: tracker.index > 0 && !s.ending ? s.prevLeg : null,
                        child: const Text('이전 구간'),
                      ),
                      const SizedBox(width: 8),
                      OutlinedButton(
                        onPressed: !tracker.isLast && !s.ending ? s.nextLeg : null,
                        child: const Text('다음 구간'),
                      ),
                      const Spacer(),
                      FilledButton.icon(
                        onPressed: s.ending ? null : _end,
                        icon: const Icon(Icons.stop),
                        label: const Text('안내 종료'),
                      ),
                    ],
                  ),
                  const SizedBox(height: 4),
                  if (s.activityMismatch)
                    Text(
                      '감지된 활동(${ActivityClassifier.label(s.activity)})이 이 구간과 다릅니다. '
                      '구간을 넘기거나 안내를 종료하세요 — 이 동안의 샘플은 속도 학습에서 뺍니다.',
                      style: TextStyle(color: Theme.of(context).colorScheme.error),
                    ),
                  Text(
                    '${s.status} · 샘플 ${s.samples}'
                    '${s.activityOn ? ' · 활동 ${ActivityClassifier.label(s.activity)}' : ''}'
                    '${s.accuracyM == null ? '' : ' · 정확도 ${s.accuracyM!.round()}m'}'
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
    );
  }
}
