import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter_activity_recognition/flutter_activity_recognition.dart';
import 'package:geolocator/geolocator.dart';
import 'package:latlong2/latlong.dart';

import '../api/client.dart';
import '../models/itinerary.dart';
import '../models/leg_detail.dart';
import '../models/place.dart';
import '../models/plan_request.dart';
import '../models/trace_sample.dart';
import '../settings/settings_store.dart';
import '../util/hhmm.dart';
import 'activity_classifier.dart';
import 'arrival_detector.dart';
import 'background_location.dart';
import 'geo.dart' as geo;
import 'guide_overlay.dart';
import 'guide_store.dart';
import 'instruction.dart';
import 'leg_tracker.dart';
import 'off_route.dart';
import 'status_notification.dart';
import 'step_tracker.dart';
import 'stop_tracker.dart';
import 'trace_uploader.dart';
import 'tts_speaker.dart';
import 'voice_guide.dart';

/// 안내 한 번의 상태. 위치 스트림·구간 추적·음성·알림창·궤적 업로드를 들고 있으며 화면과 떨어져 있다 —
/// 안내 화면을 닫아도 살아 있어 다른 경로를 찾아보는 동안에도 안내가 이어진다. 끝내는 것은 end() 뿐이다.
/// 화면은 이 객체를 듣기만 하고(ChangeNotifier) 지도·버튼만 그린다.
class GuideSession extends ChangeNotifier {
  GuideSession({
    required this.api,
    required this.request,
    required this.itinerary,
    this.speak,
    this.store = const GuideStore(),
    DateTime Function()? clock,
  }) : _clock = clock,
       _resumedTripId = null,
       startedAt = (clock ?? DateTime.now)() {
    tracker = LegTracker(itinerary.legs, now: now());
    _initTrackers();
  }

  /// 디스크에 남겨 둔 안내를 이어받는다(앱이 죽었다 다시 켜진 경우). start() 는 trip 을 새로 내지 않고
  /// 저장된 trip 에 이어 올린다.
  GuideSession.resume(
    GuideSnapshot snap, {
    required this.api,
    this.speak,
    this.store = const GuideStore(),
  }) : request = snap.request,
       itinerary = snap.itinerary,
       _clock = null,
       _resumedTripId = snap.tripId,
       startedAt = snap.startedAt {
    tracker = LegTracker.resume(
      snap.legs,
      index: snap.legIndex,
      shift: snap.shift,
    );
    _initTrackers();
  }

  void _initTrackers() {
    _steps = _stepTrackerFor(tracker.current);
    _stops = StopTracker(
      tracker.current,
      tracker.currentPoints,
      shift: tracker.shift,
    );
    _remainingStops = tracker.current.transitLeg
        ? tracker.current.stops.length + 1
        : 0;
    _instr = _buildInstruction();
  }

  /// 위치 요청 간격 2초: 화면의 내 위치와 구간 넘김이 바로 따라오게. 서버로 올리는 표본은 TraceUploader 가
  /// minGap(5초)으로 솎는다 — 서버 speed 패키지의 표본 수 기준이 5초 간격을 전제한다.
  static const sampleInterval = Duration(seconds: 2);

  /// 위치가 아예 끊기는 지하에서도 시간표로 구간을 넘기려고 이 간격으로 한 번씩 본다.
  static const tickInterval = Duration(seconds: 10);

  /// 경로 이탈 재탐색을 이 간격보다 자주 하지 않는다. 다시 찾은 경로에서도 벗어나면 서버 호출이 이어진다.
  static const rerouteMinGap = Duration(minutes: 1);

  final ApiClient api;
  final PlanRequest request;
  final Itinerary itinerary;

  /// 음성 안내 발화. 기본은 폰 내장 음성(TtsSpeaker)이고 테스트에서 바꿔 끼운다.
  final Speak? speak;

  /// 진행 중인 안내를 디스크에 남기는 곳. 앱이 죽어도 다시 켤 때 이어받는다.
  final GuideStore store;
  final DateTime Function()? _clock;

  /// 이어받은 안내의 서버 trip. null 이면 start() 가 새로 발급한다.
  final String? _resumedTripId;

  /// 안내를 시작한 시각. "현재 경로" 탭이 언제 시작했는지 보여 준다.
  final DateTime startedAt;

  /// 지금 어느 구간인가. 화면이 경로선·현재 구간을 그릴 때 읽는다.
  late final LegTracker tracker;

  late StepTracker _steps;
  late StopTracker _stops;
  late Instruction _instr;
  late int _remainingStops;
  final GuideStatusNotification _statusNotification = GuideStatusNotification();
  final ActivityClassifier _activity = ActivityClassifier();
  final OffRouteDetector _offRoute = OffRouteDetector();
  final ArrivalDetector _arrival = ArrivalDetector();
  final VoiceGuide _voice = VoiceGuide(enabled: false);
  late final DateTime? _eta = DateTime.tryParse(itinerary.end);
  Timer? _ticker;
  TtsSpeaker? _speaker;
  TraceUploader? _uploader;
  StreamSubscription<Position>? _positions;
  StreamSubscription<Activity>? _activities;
  bool _activityOn = false;
  (String, String)? _rawActivity;
  LatLng? _here;
  double? _accuracyM;
  int _samples = 0;
  String _status = '준비 중…';
  bool _ending = false;
  bool _rerouting = false;
  DateTime? _lastReroute;
  bool _ended = false;
  bool _closed = false;
  bool _overlayEnabled = false;
  final Map<String, String?> _landmarks = {};
  final Set<String> _landmarksRequested = {};

  Instruction get instr => _instr;
  LatLng? get here => _here;
  double? get accuracyM => _accuracyM;
  int get samples => _samples;
  String get status => _status;
  bool get ending => _ending;

  /// 안내가 끝났다. "현재 경로" 탭은 이때 비운다.
  bool get ended => _ended;
  bool get activityOn => _activityOn;
  String get activity => _activity.current;
  TraceUploader? get uploader => _uploader;

  /// 구간 수단과 감지된 활동이 다르다(그 동안의 샘플은 속도 학습에서 빠진다).
  bool get activityMismatch =>
      _activityOn &&
      ActivityClassifier.mismatch(tracker.mode, _activity.current);

  /// 도착 예정 시각. 놓친 열차만큼 밀린 시간(LegTracker.shift)을 더한다.
  DateTime? get eta => _eta?.add(tracker.shift);

  /// 남은 시간(분). 도착 예정 시각까지 남은 값이다.
  int get remainMin {
    final at = eta;
    if (at == null) return 0;
    final left = at.difference(now()).inSeconds;
    return left <= 0 ? 0 : (left / 60).round();
  }

  DateTime now() => (_clock ?? DateTime.now)();

  /// 위치 권한·스트림·trip 발급·음성·활동 인식을 켠다. 실패하면 status 에 이유가 남고 안내는 시작되지 않는다.
  Future<void> start() async {
    var perm = await Geolocator.checkPermission();
    if (perm == LocationPermission.denied) {
      perm = await Geolocator.requestPermission();
    }
    if (_closed) return;
    if (perm == LocationPermission.denied ||
        perm == LocationPermission.deniedForever) {
      _set(() => _status = '위치 권한이 없어 안내를 시작할 수 없습니다. 설정에서 허용한 뒤 다시 시작하세요.');
      return;
    }
    if (!await Geolocator.isLocationServiceEnabled()) {
      _set(() => _status = '기기 위치 서비스가 꺼져 있습니다.');
      return;
    }
    // 거부해도 안내는 계속한다(알림창에 안내 알림만 안 뜬다).
    await requestNotificationPermission();
    if (_closed) return;
    // 위치 포그라운드 서비스는 앱이 백그라운드로 간 뒤에는 시작되지 않는다(Android 12+,
    // ForegroundServiceStartNotAllowedException). 스트림은 trip 발급을 기다리기 전에 연다.
    // 발급 전 샘플은 서버에 올리지 않는다.
    _positions = Geolocator.getPositionStream(
      locationSettings: guideLocationSettings(sampleInterval),
    ).listen(_onPosition, onError: (e) => _set(() => _status = '위치 오류: $e'));
    try {
      final tripId = _resumedTripId ?? await api.startTrip();
      if (_closed) {
        // trip 발급을 기다리는 동안 안내를 놓았다면 서버에 열린 기록을 남기지 않는다(이어받은 trip 은 그대로 둔다).
        if (_resumedTripId == null) {
          try {
            await api.endTrip(tripId);
          } catch (_) {
            // 이미 놓은 안내라 정리는 최선 노력으로 끝낸다.
          }
        }
        return;
      }
      _uploader = TraceUploader(api: api, tripId: tripId)..start();
    } catch (e) {
      await _positions?.cancel();
      _positions = null;
      _set(() => _status = 'trip 발급 실패: $e');
      return;
    }
    _persist();
    _set(() => _status = '안내 중');
    _ticker = Timer.periodic(tickInterval, (_) => _onTick());
    // 첫 발화부터 랜드마크를 쓰되 외부 검색 때문에 안내 시작이 지연되지 않게 2초만 기다린다.
    // 늦게 도착한 결과는 진행 중인 안내에 비동기로 반영하고, 실패하면 거리 안내로 계속한다.
    final firstLandmarks = _prefetchLandmarks();
    try {
      await firstLandmarks.timeout(const Duration(seconds: 2));
    } on TimeoutException {
      unawaited(_refreshLandmarks(firstLandmarks));
    }
    if (_closed) return;
    _instr = _buildInstruction();
    await _startVoice();
    await _startActivity();
    await refreshOverlaySetting();
  }

  /// 미니 지도 설정(켜짐·진하기)을 저장소에서 다시 읽어 이 안내에 바로 반영한다. 안내 시작 때와, 안내 중 설정 화면이
  /// 저장한 뒤에 부른다 — 설정값을 네이티브에 반영하는 곳은 여기뿐이라 켜기·끄기가 같은 경로로 반영된다
  /// (종료·폐기 때의 stop() 은 설정과 무관하게 끄는 별개 경로).
  Future<void> refreshOverlaySetting() async {
    _overlayEnabled = await SettingsStore.loadOverlayGuide();
    final overlayOpacity = await SettingsStore.loadOverlayOpacity();
    if (_closed) return;
    await GuideOverlayPlatform.setEnabled(
      _overlayEnabled,
      opacity: overlayOpacity,
    );
    _updateOverlay();
  }

  /// 음성 안내 준비. 설정이 꺼져 있거나 폰에 쓸 수 있는 음성 엔진이 없으면 소리 없이 진행한다.
  Future<void> _startVoice() async {
    if (speak != null) {
      _voice.speak = speak;
      _voice.enabled = true;
    } else {
      if (!await SettingsStore.loadVoiceGuide()) return;
      _speaker = await TtsSpeaker.create();
      if (_closed) return;
      if (_speaker == null) {
        _set(() => _status = '안내 중 · 음성 엔진 없음');
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
      if (_closed || perm != ActivityPermission.GRANTED) return;
      // 판정이 바뀔 때만 오는 스트림. 확정·정지 감쇠는 settle 시점(위치 샘플과 10초 주기 점검)에 한다.
      _activities = ar.activityStream.listen((a) {
        if (_closed) return;
        _rawActivity = (a.type.name, a.confidence.name);
        _activity.observe(a.type.name, a.confidence.name, DateTime.now());
      }, onError: (_) {});
      _set(() => _activityOn = true);
    } catch (_) {
      // 플러그인·Play 서비스 없음 등: 활동 없이 진행
    }
  }

  void _onPosition(Position p) {
    if (_closed || _ending) return;
    final at = now();
    var refreshLandmarks = false;
    _activity.settle(at);
    final here = LatLng(p.latitude, p.longitude);
    if (tracker.update(
      p.latitude,
      p.longitude,
      accuracyM: p.accuracy,
      now: at,
    )) {
      _offRoute.reset();
      _enterLeg();
    } else {
      if (_steps.update(p.latitude, p.longitude, accuracyM: p.accuracy)) {
        refreshLandmarks = true;
      }
      _checkOffRoute(p, at);
    }
    _remainingStops = tracker.current.transitLeg
        ? _stops.remaining(p.latitude, p.longitude, p.accuracy, at)
        : 0;
    _uploader?.add(
      TraceSample(
        ts: p.timestamp,
        lat: p.latitude,
        lon: p.longitude,
        accuracyM: p.accuracy,
        mode: tracker.mode,
        activity: _activityOn ? _activity.current : null,
        activityRaw: _rawActivity?.$1,
        activityConf: _rawActivity?.$2,
      ),
    );
    _here = here;
    _accuracyM = p.accuracy;
    _samples++;
    _instr = _buildInstruction(here: here);
    _announce();
    _notify();
    if (refreshLandmarks) unawaited(_refreshLandmarks());
    _checkArrived(p);
  }

  /// 마지막 구간에서 목적지에 닿았으면 사용자가 누르지 않아도 끝낸다.
  void _checkArrived(Position p) {
    if (!tracker.isLast || _ending || _ended) return;
    final leg = tracker.current;
    if (!_arrival.update(
      geo.distanceM(p.latitude, p.longitude, leg.toLat, leg.toLon),
      p.accuracy,
    )) {
      return;
    }
    unawaited(_autoEnd());
  }

  /// 도착을 알리고 안내를 끝낸다. 실패하면 상태줄에 사유가 남고 사용자가 종료를 누른다 — ArrivalDetector 가
  /// 한 번만 참을 주므로 표본마다 다시 시도하지 않는다.
  Future<void> _autoEnd() async {
    await _voice.say('목적지에 도착했습니다', cueKey: 'arrived');
    if (await end() != null) _set(() => _status = '목적지에 도착해 안내를 끝냈습니다');
  }

  /// 위치가 끊긴 동안에도 시간표로 구간·남은 정거장을 따라간다(지하).
  void _onTick() {
    if (_closed || _ending) return;
    final at = now();
    // 위치가 아예 끊긴 지하에서도 활동 판정이 흐르게 한다 — 그러지 않으면 정지 감쇠가 멈춰 직전 활동이 화면에 박힌다.
    _activity.settle(at);
    if (tracker.tick(at)) _enterLeg();
    if (tracker.current.transitLeg) {
      _remainingStops = _stops.remaining(null, null, 0, at);
    }
    _instr = _buildInstruction();
    _announce();
    _notify();
  }

  /// 도보 구간에서 경로를 벗어났으면 그 구간을 현재 위치에서 다시 찾는다. 대중교통은 정해진 노선을 따라가고
  /// 자전거는 양끝이 대여소로 묶여 있어(어디로 달리든 대여소에 반납한다) 둘 다 경로 이탈이 성립하지 않는다.
  void _checkOffRoute(Position p, DateTime at) {
    if (_rerouting || _ending || tracker.current.mode != 'WALK') return;
    final away = geo
        .projectOnPolyline(p.latitude, p.longitude, tracker.currentPoints)
        .distM;
    if (!_offRoute.update(away, p.accuracy)) return;
    final last = _lastReroute;
    if (last != null && at.difference(last) < rerouteMinGap) {
      _offRoute.reset(); // 제한에 걸려 버린 신호라 다음 표본부터 다시 센다
      return;
    }
    _lastReroute = at;
    _replanLeg(LatLng(p.latitude, p.longitude));
  }

  /// 현재 위치에서 이 구간의 도착지까지 도보로 다시 찾아 구간을 갈아 끼운다. 뒤 구간은 그대로 둔다 —
  /// 다시 찾은 결과로 뒤 탑승을 놓치는 경우는 보지 않는다.
  Future<void> _replanLeg(LatLng from) async {
    final leg = tracker.current;
    _set(() {
      _rerouting = true;
      _status = '경로를 벗어나 다시 찾는 중…';
    });
    try {
      final res = await api.plan(
        PlanRequest(
          origin: Place(
            name: '현재 위치',
            address: '',
            lat: from.latitude,
            lon: from.longitude,
          ),
          destination: Place(
            name: leg.toName,
            address: '',
            lat: leg.toLat,
            lon: leg.toLon,
          ),
          segmentModes: const [SegmentMode.walk],
        ),
      );
      if (_closed || _ending) return;
      // 기다리는 동안 구간이 넘어갔으면(자동 인계·버튼) 이 결과는 남의 구간 것이다. 이탈 셈은 구간이 바뀔 때
      // 이미 지워졌다.
      if (!identical(leg, tracker.current)) {
        _set(() => _status = '안내 중');
        return;
      }
      final fresh = res.itineraries.isEmpty
          ? const <Leg>[]
          : res.itineraries.first.legs;
      if (fresh.isEmpty) {
        _offRoute.reset();
        _set(() => _status = '다시 찾은 경로가 없습니다 — 원래 경로로 안내합니다');
        return;
      }
      tracker.replaceCurrent(fresh, now());
      _offRoute.reset();
      _voice.forget('L${tracker.index}:');
      await _voice.say(
        '경로를 벗어나 다시 찾았습니다',
        cueKey: 'reroute:${now().millisecondsSinceEpoch}',
      );
      _enterLeg();
      _set(() => _status = '경로를 다시 찾았습니다');
    } catch (e) {
      _offRoute.reset();
      _set(() => _status = '재탐색 실패: $e. 원래 경로로 안내합니다');
    } finally {
      _set(() => _rerouting = false);
    }
  }

  /// 구간이 바뀌었을 때(자동·버튼 공통) 단계·정차 추적을 새 구간으로 갈아 끼우고 문구를 다시 만든다.
  void _enterLeg() {
    final leg = tracker.current;
    _arrival.reset(); // 마지막 구간을 벗어났거나 갈아 끼웠으면 도착 셈을 다시 시작한다
    _steps = _stepTrackerFor(leg);
    _stops = StopTracker(leg, tracker.currentPoints, shift: tracker.shift);
    _remainingStops = leg.transitLeg ? leg.stops.length + 1 : 0;
    _instr = _buildInstruction();
    unawaited(_refreshLandmarks());
    _announce();
    _persist(); // 구간·밀린 시간이 바뀔 때만 남기면 된다(나머지는 안내 내내 그대로다)
  }

  /// 진행 중인 안내를 디스크에 남긴다. trip 발급 전이거나 끝나는 중이면 남길 것이 없다.
  void _persist() {
    final up = _uploader;
    if (up == null || _ending || _ended || _closed) return;
    unawaited(
      store.save(
        GuideSnapshot(
          tripId: up.tripId,
          request: request,
          itinerary: itinerary,
          legs: tracker.legs,
          legIndex: tracker.index,
          shift: tracker.shift,
          startedAt: startedAt,
        ),
      ),
    );
  }

  /// 안내가 끝났다. 끝난 안내는 되살리지 않는다.
  void _finished() {
    _set(() => _ended = true);
    unawaited(GuideOverlayPlatform.stop());
    unawaited(store.clear());
  }

  /// 사용자가 손으로 앞뒤 구간을 맞춘다.
  void prevLeg() {
    if (tracker.index == 0 || _ending) return;
    tracker.prev(now: now());
    _offRoute.reset();
    _voice.forget('L${tracker.index}:');
    _enterLeg();
    _notify();
  }

  void nextLeg() {
    if (tracker.isLast || _ending) return;
    tracker.next(now: now());
    _offRoute.reset();
    _voice.forget('L${tracker.index}:');
    _enterLeg();
    _notify();
  }

  /// 지금 안내를 읽고(같은 안내 시점은 한 번만) 알림창 내용을 맞춘다.
  void _announce() {
    _voice.say(_instr.utterance, cueKey: _instr.cueKey);
    // 알림창은 접힌 상태에서 제목 한 줄만 보이므로 남은 시간을 제목 앞에 둔다.
    final at = eta;
    final when = at == null ? '' : '도착 예정 ${hhmm(at.toLocal())} · ';
    _statusNotification.show(
      '남은 $remainMin분 · ${_instr.now}',
      '$when다음: ${_instr.next}',
    );
  }

  StepTracker _stepTrackerFor(Leg leg) =>
      StepTracker(leg.steps, endLat: leg.toLat, endLon: leg.toLon);

  String _landmarkKey(WalkStep step) =>
      '${step.lat.toStringAsFixed(5)},${step.lon.toStringAsFixed(5)}';

  Future<void> _prefetchLandmarks() async {
    final leg = tracker.current;
    if (leg.transitLeg || leg.steps.isEmpty) return;
    final targets = <WalkStep>[];
    var from = _steps.index;
    while (targets.length < 3) {
      final index = nextTurnStepIndex(leg.steps, from);
      if (index < 0) break;
      targets.add(leg.steps[index]);
      from = index;
    }
    await Future.wait(
      targets.map((step) async {
        final key = _landmarkKey(step);
        if (!_landmarksRequested.add(key)) return;
        for (var attempt = 0; attempt < 2; attempt++) {
          try {
            _landmarks[key] = (await api.landmark(step.lat, step.lon))?.name;
            return;
          } catch (_) {
            if (attempt == 0) continue;
            _landmarks.remove(key);
            _landmarksRequested.remove(key);
          }
        }
      }),
    );
  }

  Future<void> _refreshLandmarks([Future<void>? pending]) async {
    final before = _instr;
    final hadLandmark = _currentTurnLandmark().isNotEmpty;
    await (pending ?? _prefetchLandmarks());
    if (_closed || _ending) return;
    final after = _buildInstruction();
    // 같은 회전인데 랜드마크 이름이 새로 붙었을 때만 다시 읽는다. 거리(now)는 걷기만 해도 바뀌므로 비교 기준이 못 된다 —
    // 결과가 null 이거나 실패한 늦은 조회는 같은 문장을 두 번 읽게 했다.
    if (after.cueKey != before.cueKey || hadLandmark || _currentTurnLandmark().isEmpty) return;
    _instr = after;
    // 현재 회전 조회가 늦게 끝난 경우에만 보강 문장을 한 번 읽는다. 평소에는 앞선 단계에서 미리 받아 이 경로를 타지 않는다.
    await _voice.say(after.utterance, cueKey: '${after.cueKey}:landmark');
    _notify();
  }

  /// 현재 구간의 다음 회전점에 붙은 랜드마크 이름. 없으면 빈 문자열.
  String _currentTurnLandmark() {
    final leg = tracker.current;
    if (leg.transitLeg || leg.steps.isEmpty) return '';
    final turn = nextTurnStepIndex(leg.steps, _steps.index);
    return turn >= 0 ? _landmarks[_landmarkKey(leg.steps[turn])] ?? '' : '';
  }

  Instruction _buildInstruction({LatLng? here}) {
    final at = here ?? _here;
    final landmark = _currentTurnLandmark();
    return buildInstruction(
      request: request,
      itinerary: itinerary,
      legIndex: tracker.index,
      legs: tracker.legs,
      stepIndex: _steps.index,
      stepRemainM: at == null || _steps.isEmpty
          ? null
          : _steps.remainM(at.latitude, at.longitude),
      remainingStops: _remainingStops,
      nextStopName: _stops.nextStopName(),
      landmark: landmark,
    );
  }

  /// 안내를 끝낸다. 남은 샘플을 보내고 trip 을 닫아 서버가 이번 안내의 속도를 프로파일에 반영하게 한다.
  /// 성공하면 종료 응답(요약용), 보낼 샘플이 남았거나 실패하면 null 이고 status 에 이유가 남는다.
  Future<Map<String, dynamic>?> end() async {
    if (_ending) return null;
    final up = _uploader;
    _voice.enabled = false;
    _ticker?.cancel();
    _ticker = null;
    await _speaker?.stop();
    await _statusNotification.cancel();
    _set(() {
      _ending = true;
      _status = '샘플 전송 중…';
    });
    await _positions?.cancel();
    _positions = null;
    // 화면이 닫힌 채 스스로 끝나면(목적지 도착) 아무도 dispose 를 부르지 않는다 — 여기서 같이 끊는다.
    await _activities?.cancel();
    _activities = null;
    if (up == null) {
      _finished();
      return const {};
    }
    await up.flush();
    if (up.pending > 0) {
      _set(() {
        _ending = false;
        _status =
            '샘플 ${up.pending}개 전송 실패(${up.lastError}). 다시 종료를 누르면 재전송합니다.';
      });
      return null;
    }
    try {
      final res = await api.endTrip(up.tripId);
      _finished();
      return res;
    } on ApiException catch (e) {
      // 409 = 서버가 이미 이 trip 을 닫고 학습까지 반영한 상태(앞선 종료 요청의 응답만 잃은 경우). 완료로 본다.
      if (e.status == 409) {
        _set(() => _status = '이미 종료된 안내입니다.');
        _finished();
        return const {};
      }
      _endFailed(e);
      return null;
    } catch (e) {
      _endFailed(e);
      return null;
    }
  }

  void _endFailed(Object e) => _set(() {
    _ending = false;
    _status = '종료 실패: $e. 다시 종료를 누르세요.';
  });

  void _set(void Function() change) {
    if (_closed) return;
    change();
    _notify();
  }

  void _notify() {
    if (!_closed) {
      notifyListeners();
      _updateOverlay();
    }
  }

  void _updateOverlay() {
    if (!_overlayEnabled || _closed || _ended) return;
    final leg = tracker.current;
    LatLng? turn;
    if (!leg.transitLeg && leg.steps.isNotEmpty) {
      final index = nextTurnStepIndex(leg.steps, _steps.index);
      if (index >= 0) turn = LatLng(leg.steps[index].lat, leg.steps[index].lon);
    }
    turn ??= LatLng(leg.toLat, leg.toLon);
    var color = int.tryParse(leg.color, radix: 16);
    color = color == null
        ? switch (leg.mode) {
            'WALK' => 0xFF616161,
            'BICYCLE' => 0xFF2E7D32,
            'BUS' => 0xFF1565C0,
            'SUBWAY' || 'RAIL' => 0xFFD84315,
            _ => 0xFF6A1B9A,
          }
        : 0xFF000000 | color;
    unawaited(
      GuideOverlayPlatform.update(
        GuideOverlaySnapshot(
          baseUrl: api.baseUrl,
          token: api.token,
          points: tracker.currentPoints,
          here: _here,
          turn: turn,
          now: _instr.now,
          next: _instr.next,
          remainMin: remainMin,
          eta: eta == null ? '' : hhmm(eta!.toLocal()),
          color: color,
        ),
      ),
    );
  }

  /// 안내를 놓는다(화면이 아니라 세션 자체를 버릴 때). end() 와 달리 서버의 trip 을 닫지 않는다.
  @override
  void dispose() {
    _closed = true;
    // 놓은 안내는 되살리지 않는다. 새 안내로 갈아타는 경우 ActiveGuide.set 이 이 dispose 를 먼저 부르고
    // 새 세션은 start() 뒤에 남기므로, 여기서 지워도 새 것을 지우지 않는다.
    unawaited(store.clear());
    _positions?.cancel();
    _activities?.cancel();
    _uploader?.dispose();
    _voice.enabled = false;
    _ticker?.cancel();
    _speaker?.stop();
    _statusNotification.cancel();
    GuideOverlayPlatform.stop();
    super.dispose();
  }
}
