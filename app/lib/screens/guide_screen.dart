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
import '../guide/instruction.dart';
import '../guide/leg_tracker.dart';
import '../guide/status_notification.dart';
import '../guide/step_tracker.dart';
import '../guide/stop_tracker.dart';
import '../guide/trace_uploader.dart';
import '../guide/tts_speaker.dart';
import '../guide/voice_guide.dart';
import '../models/itinerary.dart';
import '../models/plan_request.dart';
import '../models/trace_sample.dart';
import '../settings/settings_store.dart';
import '../util/leg_names.dart';
import '../widgets/guide_card.dart';
import '../widgets/mode_icon.dart';

/// 안내 화면. 위치를 받아 현재 구간·단계를 따라가며 상단 카드에 지금 할 일을 보여 주고(설정에 따라 읽어 주고),
/// 서버 trip 에 샘플을 올린다. 위치는 포그라운드 서비스(상단 알림)로 받아 화면이 꺼지거나 다른 앱으로 넘어가도 이어진다.
/// "안내 종료" 를 누르면 남은 샘플을 보내고 trip 을 닫는다 — 서버가 그때 이번 안내의 걷기·자전거 속도를 내 프로파일에 반영한다.
class GuideScreen extends StatefulWidget {
  const GuideScreen({
    super.key,
    required this.api,
    required this.request,
    required this.itinerary,
    this.speak,
    this.clock,
  });

  final ApiClient api;
  final PlanRequest request;
  final Itinerary itinerary;
  // 음성 안내 발화. 기본은 폰 내장 음성(TtsSpeaker)이고 테스트에서 바꿔 끼운다.
  final Speak? speak;
  // 현재 시각. 시간표로 구간을 넘기는 판정에 쓰며 테스트에서 바꿔 끼운다.
  final DateTime Function()? clock;

  @override
  State<GuideScreen> createState() => _GuideScreenState();
}

class _GuideScreenState extends State<GuideScreen> {
  // 위치 요청 간격 2초: 화면의 내 위치와 구간 넘김이 바로 따라오게. 서버로 올리는 표본은 TraceUploader 가
  // minGap(5초)으로 솎는다 — 서버 speed 패키지의 표본 수 기준이 5초 간격을 전제한다.
  static const sampleInterval = Duration(seconds: 2);
  // 따라가기 모드에서 지도를 이 배율 밑으로는 줄이지 않는다(도보 안내에서 모퉁이가 보이는 정도).
  static const followZoom = 17.0;

  // 위치가 아예 끊기는 지하에서도 시간표로 구간을 넘기려고 이 간격으로 한 번씩 본다.
  static const tickInterval = Duration(seconds: 10);

  late final LegTracker _tracker = LegTracker(widget.itinerary.legs, now: _now());
  late StepTracker _steps = _stepTrackerFor(_tracker.current);
  late StopTracker _stops = StopTracker(_tracker.current, _tracker.currentPoints, shift: _tracker.shift);
  final GuideStatusNotification _statusNotification = GuideStatusNotification();
  Timer? _ticker;
  late final DateTime? _eta = DateTime.tryParse(widget.itinerary.end);
  final ActivityClassifier _activity = ActivityClassifier();
  final MapController _map = MapController();
  final VoiceGuide _voice = VoiceGuide(enabled: false);
  TtsSpeaker? _speaker;
  TraceUploader? _uploader;
  StreamSubscription<Position>? _positions;
  StreamSubscription<Activity>? _activities;
  bool _activityOn = false; // 활동 인식 권한을 받아 스트림을 켰는지. 아니면 샘플에 activity 를 싣지 않는다
  (String, String)? _rawActivity; // 활동 인식 원시 판정·신뢰도(서버 진단용)
  LatLng? _here;
  double? _accuracyM;
  int _samples = 0;
  // 첫 구간이 대중교통이면 첫 위치 표본 전에도 정거장 수를 세 두어야 한다(0 이면 곧바로 하차 안내가 나간다).
  late int _remainingStops = _tracker.current.transitLeg ? _tracker.current.stops.length + 1 : 0;
  String _status = '준비 중…';
  bool _ending = false;
  bool _follow = true; // 지도가 내 위치를 따라간다. 손으로 지도를 옮기면 꺼진다
  bool _mapReady = false;
  late Instruction _instr = _buildInstruction();

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
    _ticker = Timer.periodic(tickInterval, (_) => _onTick());
    await _startVoice();
    await _startActivity();
  }

  /// 음성 안내 준비. 설정이 꺼져 있거나 폰에 쓸 수 있는 음성 엔진이 없으면 소리 없이 진행한다.
  Future<void> _startVoice() async {
    if (widget.speak != null) {
      _voice.speak = widget.speak;
      _voice.enabled = true;
    } else {
      if (!await SettingsStore.loadVoiceGuide()) return;
      _speaker = await TtsSpeaker.create();
      if (!mounted) return;
      if (_speaker == null) {
        setState(() => _status = '안내 중 · 음성 엔진 없음');
        return;
      }
      _voice.speak = _speaker!.speak;
      _voice.enabled = true;
    }
    await _voice.say(_instr.utterance, cueKey: _instr.cueKey);
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
          _rawActivity = (a.type.name, a.confidence.name);
          _activity.observe(a.type.name, a.confidence.name, DateTime.now());
        }
      }, onError: (_) {});
      setState(() => _activityOn = true);
    } catch (_) {
      // 플러그인·Play 서비스 없음 등: 활동 없이 진행
    }
  }

  DateTime _now() => (widget.clock ?? DateTime.now)();

  void _onPosition(Position p) {
    if (!mounted || _ending) return;
    final now = _now();
    _activity.settle(now);
    final here = LatLng(p.latitude, p.longitude);
    final changed = _tracker.update(p.latitude, p.longitude, accuracyM: p.accuracy, now: now);
    if (changed) {
      _enterLeg();
    } else {
      _steps.update(p.latitude, p.longitude, accuracyM: p.accuracy);
    }
    _remainingStops =
        _tracker.current.transitLeg ? _stops.remaining(p.latitude, p.longitude, p.accuracy, now) : 0;
    _uploader?.add(TraceSample(
      ts: p.timestamp,
      lat: p.latitude,
      lon: p.longitude,
      accuracyM: p.accuracy,
      mode: _tracker.mode,
      activity: _activityOn ? _activity.current : null,
      activityRaw: _rawActivity?.$1,
      activityConf: _rawActivity?.$2,
    ));
    setState(() {
      _here = here;
      _accuracyM = p.accuracy;
      _samples++;
      _instr = _buildInstruction(here: here);
    });
    _announce();
    if (_follow && _mapReady) {
      _map.move(here, _map.camera.zoom < followZoom ? followZoom : _map.camera.zoom);
    }
  }

  /// 위치가 끊긴 동안에도 시간표로 구간·남은 정거장을 따라간다(지하).
  void _onTick() {
    if (!mounted || _ending) return;
    final now = _now();
    if (_tracker.tick(now)) _enterLeg();
    if (_tracker.current.transitLeg) _remainingStops = _stops.remaining(null, null, 0, now);
    setState(() => _instr = _buildInstruction());
    _announce();
  }

  /// 구간이 바뀌었을 때(자동·버튼 공통) 단계·정차 추적을 새 구간으로 갈아 끼우고 문구를 다시 만든다.
  void _enterLeg() {
    final leg = _tracker.current;
    _steps = _stepTrackerFor(leg);
    _stops = StopTracker(leg, _tracker.currentPoints, shift: _tracker.shift);
    _remainingStops = leg.transitLeg ? leg.stops.length + 1 : 0;
    _instr = _buildInstruction();
    _announce();
  }

  /// 지금 안내를 읽고(같은 안내 시점은 한 번만) 알림창 내용을 맞춘다.
  void _announce() {
    _voice.say(_instr.utterance, cueKey: _instr.cueKey);
    // 알림창은 접힌 상태에서 제목 한 줄만 보이므로 남은 시간을 제목 앞에 둔다.
    final at = _etaNow;
    final eta = at == null ? '' : '도착 예정 ${GuideCard.hhmm(at.toLocal())} · ';
    _statusNotification.show('남은 $_remainMin분 · ${_instr.now}', '$eta다음: ${_instr.next}');
  }

  /// 도착 예정 시각. 놓친 열차만큼 밀린 시간(LegTracker.shift)을 더한다.
  DateTime? get _etaNow => _eta?.add(_tracker.shift);

  StepTracker _stepTrackerFor(Leg leg) =>
      StepTracker(leg.steps, endLat: leg.toLat, endLon: leg.toLon);

  Instruction _buildInstruction({LatLng? here}) {
    final at = here ?? _here;
    return buildInstruction(
      request: widget.request,
      itinerary: widget.itinerary,
      legIndex: _tracker.index,
      stepIndex: _steps.index,
      stepRemainM: at == null || _steps.isEmpty ? null : _steps.remainM(at.latitude, at.longitude),
      remainingStops: _remainingStops,
      nextStopName: _stops.nextStopName(),
    );
  }

  /// 남은 시간(분). 도착 예정 시각까지 남은 값이다.
  int get _remainMin {
    final at = _etaNow;
    if (at == null) return 0;
    final left = at.difference(_now()).inSeconds;
    return left <= 0 ? 0 : (left / 60).round();
  }

  Future<void> _end() async {
    final up = _uploader;
    _voice.enabled = false;
    _ticker?.cancel();
    await _speaker?.stop();
    await _statusNotification.cancel();
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
    _voice.enabled = false;
    _ticker?.cancel();
    _speaker?.stop();
    _statusNotification.cancel();
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
      final pts = _tracker.points[i];
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
          GuideCard(
            icon: modeIcon(leg),
            color: modeColor(leg),
            eta: _etaNow,
            remainMin: _remainMin,
            now: _instr.now,
            next: _instr.next,
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
                        '${remainM == null ? '' : ' · 끝까지 ${remainM.round()}m'}'
                        '${leg.fastExitLabel == null ? '' : '\n${leg.fastExitLabel}'}'),
                  ),
                  Row(
                    children: [
                      OutlinedButton(
                        onPressed: _tracker.index > 0 && !_ending ? () => setState(() {
                              _tracker.prev(now: _now());
                              _voice.forget('L${_tracker.index}:');
                              _enterLeg();
                            }) : null,
                        child: const Text('이전 구간'),
                      ),
                      const SizedBox(width: 8),
                      OutlinedButton(
                        onPressed: !_tracker.isLast && !_ending ? () => setState(() {
                              _tracker.next(now: _now());
                              _voice.forget('L${_tracker.index}:');
                              _enterLeg();
                            }) : null,
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
