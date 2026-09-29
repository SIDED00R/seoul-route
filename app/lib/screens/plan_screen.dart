import 'package:flutter/material.dart';

import '../api/client.dart';
import '../location/current_location.dart';
import '../models/place.dart';
import '../models/favorite_place.dart';
import '../models/plan_request.dart';
import '../models/plan_time.dart';
import '../settings/settings_store.dart';
import '../widgets/plan_time_picker.dart';
import '../widgets/via_stay_picker.dart';
import 'place_search_screen.dart';
import 'favorite_places_screen.dart';
import 'results_screen.dart';

/// 최근 경로에서 고른 검색. 같은 값이 다시 와도 입력을 덮어쓰도록 새 객체인지로 구분한다.
class PlanScreenPreset {
  const PlanScreenPreset(this.request);

  final PlanRequest request;
}

/// 길찾기 탭: 출발·경유(최대 5)·도착을 고르고 구간마다 수단을 고정한 뒤 경로를 요청한다.
/// 출발지는 현재 위치로도 고를 수 있고, 출발·도착을 맞바꿀 수 있다. 지금 출발 대신 출발·도착 시각을 고를 수 있다.
class PlanScreen extends StatefulWidget {
  const PlanScreen({
    super.key,
    required this.settings,
    this.locate = currentPlace,
    this.locateOnOpen,
    this.settingsStore = const SettingsStore(),
    this.preset,
    this.onOpenSettings,
    this.now = DateTime.now,
  });

  final Settings settings;
  final SettingsStore settingsStore;

  /// 최근 경로에서 고른 검색. 바뀌면 입력을 그 값으로 채운다.
  final PlanScreenPreset? preset;

  /// 설정 화면 열기. 홈이 앱바를 들고 있어 여기서는 안내 카드의 탭으로만 쓴다.
  final VoidCallback? onOpenSettings;

  /// 현재 위치를 Place 로 받는다. 실패하면 LocationException. 테스트가 가짜로 바꾼다.
  final Future<Place> Function(ApiClient api) locate;

  /// 화면을 열 때(설정이 나중에 채워지면 그때) 출발지를 채울 현재 위치. null 이면 채우지 않는다(홈은 권한을 묻지 않는
  /// currentPlace 를 준다).
  final Future<Place> Function(ApiClient api)? locateOnOpen;

  /// 지금 시각. 출발·도착 시각의 기본값과 지난 시각 검사에 쓴다. 테스트가 바꾼다.
  final DateTime Function() now;

  @override
  State<PlanScreen> createState() => _PlanScreenState();
}

class _PlanScreenState extends State<PlanScreen> {
  Place? _origin;
  Place? _destination;
  final List<Place> _via = [];
  final List<int> _viaStay = []; // _via 와 같은 길이. 경유지마다 머무는 분
  final List<SegmentMode> _modes = [SegmentMode.any];
  PlanTime _time = PlanTime.now;
  bool _busy = false;
  bool _locating = false;
  bool _autoLocating = false; // 화면을 열 때 받는 현재 위치. _locating 과 달리 입력을 막지 않는다
  String _error = '';
  List<FavoritePlace>? _favorites;
  String _favoritesError = '';
  int _favoritesLoadGeneration = 0;

  static const maxVia = 5;

  @override
  void initState() {
    super.initState();
    _applyPreset();
    if (widget.settings.ready) _loadFavorites();
    final locate = widget.locateOnOpen;
    if (locate != null && widget.settings.ready && _origin == null) {
      _autoLocate(locate);
    }
  }

  /// 출발지가 비어 있으면 현재 위치로 채운다. 실패하면 조용히 비워 두고, 받는 동안 사용자가 출발지를 골랐거나 경로를
  /// 찾는 중이면 늦게 온 위치로 덮지 않는다.
  Future<void> _autoLocate(Future<Place> Function(ApiClient api) locate) async {
    _autoLocating = true;
    try {
      final p = await locate(_api);
      if (mounted && _origin == null && !_busy) setState(() => _origin = p);
    } catch (_) {
      // 버튼으로 다시 받을 수 있다
    } finally {
      if (mounted) setState(() => _autoLocating = false);
    }
  }

  @override
  void didUpdateWidget(PlanScreen old) {
    super.didUpdateWidget(old);
    if (!identical(widget.preset, old.preset)) _applyPreset();
    if (widget.settings.token != old.settings.token ||
        widget.settings.baseUrl != old.settings.baseUrl) {
      if (widget.settings.ready) {
        _loadFavorites(clear: true);
      } else {
        _favoritesLoadGeneration++;
        setState(() {
          _favorites = null;
          _favoritesError = '';
        });
      }
    }
    final locate = widget.locateOnOpen;
    if (locate != null && !old.settings.ready && widget.settings.ready && _origin == null && !_autoLocating) {
      _autoLocate(locate);
    }
  }

  /// 최근 경로에서 고른 검색으로 입력을 채운다. 시각 조건은 지금 출발로 되돌린다(시각은 경로를 고른 뒤에 바꾼다).
  void _applyPreset() {
    final p = widget.preset;
    if (p == null) return;
    setState(() {
      _origin = p.request.origin;
      _destination = p.request.destination;
      _via
        ..clear()
        ..addAll(p.request.via);
      _viaStay
        ..clear()
        ..addAll([for (var i = 0; i < p.request.via.length; i++) p.request.stayAt(i)]);
      _modes
        ..clear()
        ..addAll(
          p.request.segmentModes.isEmpty
              ? List.filled(p.request.via.length + 1, SegmentMode.any)
              : p.request.segmentModes,
        );
      _time = PlanTime.now;
      _error = '';
    });
  }

  /// 도착 시각은 경유지·구간 수단 고정이 없을 때만 서버가 받는다.
  bool get _arriveAllowed =>
      _via.isEmpty && _modes.every((m) => m == SegmentMode.any);

  /// 도착 시각을 고를 수 없게 되면 같은 시각의 출발 시각으로 바꾼다. setState 안에서 부른다.
  void _fitTime() {
    if (_time.kind == PlanTimeKind.arrive && !_arriveAllowed) {
      _time = PlanTime(PlanTimeKind.depart, _time.at);
    }
  }

  /// 출발·도착을 맞바꾼다. 경유지·체류·구간 수단은 순서를 뒤집는다.
  void _swap() => setState(() {
    final o = _origin;
    _origin = _destination;
    _destination = o;
    final via = _via.reversed.toList();
    final stay = _viaStay.reversed.toList();
    final modes = _modes.reversed.toList();
    _via
      ..clear()
      ..addAll(via);
    _viaStay
      ..clear()
      ..addAll(stay);
    _modes
      ..clear()
      ..addAll(modes);
  });

  ApiClient get _api =>
      ApiClient(baseUrl: widget.settings.baseUrl, token: widget.settings.token);

  Future<void> _loadFavorites({bool clear = false}) async {
    final generation = ++_favoritesLoadGeneration;
    final api = _api;
    if (clear) {
      setState(() {
        _favorites = null;
        _favoritesError = '';
      });
    }
    try {
      final values = await api.favoritePlaces();
      if (mounted && generation == _favoritesLoadGeneration) {
        setState(() {
          _favorites = values;
          _favoritesError = '';
        });
      }
    } catch (_) {
      if (mounted && generation == _favoritesLoadGeneration) {
        setState(() => _favoritesError = '자주 가는 곳을 불러오지 못했습니다.');
      }
    }
  }

  Future<void> _manageFavorites() async {
    await Navigator.push(
      context,
      MaterialPageRoute(builder: (_) => FavoritePlacesScreen(api: _api)),
    );
    if (mounted) await _loadFavorites();
  }

  Future<Place?> _pick(String title) => Navigator.push<Place>(
    context,
    MaterialPageRoute(
      builder: (_) => PlaceSearchScreen(api: _api, title: title),
    ),
  );

  Future<void> _plan() async {
    final o = _origin;
    final d = _destination;
    if (o == null || d == null) return;
    final at = _time.at;
    if (at != null && !at.isAfter(widget.now())) {
      setState(() => _error = '지난 시각입니다 — 출발·도착 시각을 다시 고르세요');
      return;
    }
    setState(() {
      _busy = true;
      _error = '';
    });
    final req = PlanRequest(
      origin: o,
      destination: d,
      via: List.of(_via),
      segmentModes: List.of(_modes),
      viaStayMin: List.of(_viaStay),
      bikeLimitMin: widget.settings.bikeLimitMin,
      depart: _time.depart,
      arrive: _time.arrive,
    );
    try {
      final res = await _api.plan(req);
      if (!mounted) return;
      await Navigator.push(
        context,
        MaterialPageRoute(
          builder: (_) => ResultsScreen(api: _api, request: req, result: res),
        ),
      );
    } on ApiException catch (e) {
      setState(
        () => _error = e.status == 401
            ? '토큰이 없거나 만료됨 — 설정에서 입력'
            : '실패: ${e.message}',
      );
    } catch (e) {
      setState(() => _error = '연결 실패: $e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _useCurrentLocation() async {
    setState(() {
      _locating = true;
      _error = '';
    });
    try {
      final p = await widget.locate(_api);
      if (mounted) setState(() => _origin = p);
    } on LocationException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } catch (e) {
      if (mounted) setState(() => _error = '현재 위치 실패: $e');
    } finally {
      if (mounted) setState(() => _locating = false);
    }
  }

  void _addVia(Place p) => setState(() {
    _via.add(p);
    _viaStay.add(0);
    _modes.add(SegmentMode.any);
    _fitTime();
  });

  void _removeVia(int i) => setState(() {
    _via.removeAt(i);
    _viaStay.removeAt(i);
    _modes.removeAt(i + 1);
  });

  @override
  Widget build(BuildContext context) {
    final ready =
        widget.settings.ready &&
        _origin != null &&
        _destination != null &&
        !_busy &&
        !_locating;
    // 홈이 앱바·탭을 들고 있으므로 여기서는 본문만 낸다. Scaffold 는 ListTile 이 필요로 하는 Material 바탕을 준다.
    return Scaffold(
      body: ListView(
        padding: const EdgeInsets.all(12),
        children: [
          if (!widget.settings.ready)
            Card(
              color: Theme.of(context).colorScheme.errorContainer,
              child: ListTile(
                title: const Text('서버 주소와 토큰을 먼저 설정하세요'),
                trailing: const Icon(Icons.chevron_right),
                onTap: widget.onOpenSettings,
              ),
            ),
          _placeTile(
            '출발',
            Icons.trip_origin,
            _origin,
            () async {
              if (_locating || _busy) {
                return; // 현재 위치를 받는 동안·경로 요청 중에는 출발지 검색을 열지 않는다
              }
              final p = await _pick('출발지');
              if (p != null) setState(() => _origin = p);
            },
            extra: _locating
                ? const Padding(
                    padding: EdgeInsets.all(12),
                    child: SizedBox(
                      width: 24,
                      height: 24,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    ),
                  )
                : IconButton(
                    tooltip: '현재 위치',
                    icon: const Icon(Icons.my_location),
                    onPressed: _busy
                        ? null
                        : _useCurrentLocation, // 탐색 요청 중에는 출발지를 바꾸지 않는다
                  ),
            placeholder: _autoLocating ? '현재 위치 찾는 중…' : null,
          ),
          _segmentMode(0),
          for (var i = 0; i < _via.length; i++) ...[
            ListTile(
              leading: const Icon(Icons.flag),
              title: Text('경유 ${i + 1}: ${_via[i].name}'),
              subtitle: Text(_via[i].address),
              trailing: IconButton(
                icon: const Icon(Icons.close),
                onPressed: _busy ? null : () => _removeVia(i),
              ),
            ),
            Padding(
              padding: const EdgeInsets.only(left: 16, right: 12),
              child: ViaStayPicker(
                minutes: _viaStay[i],
                onChanged: _busy ? null : (m) => setState(() => _viaStay[i] = m),
              ),
            ),
            _segmentMode(i + 1),
          ],
          if (_via.length < maxVia)
            TextButton.icon(
              onPressed: _busy
                  ? null
                  : () async {
                      final p = await _pick('경유지 ${_via.length + 1}');
                      if (p != null) _addVia(p);
                    },
              icon: const Icon(Icons.add),
              label: const Text('경유지 추가'),
            ),
          _placeTile(
            '도착',
            Icons.place,
            _destination,
            () async {
              if (_busy) return; // 경로 요청 중에는 도착지 검색을 열지 않는다
              final p = await _pick('도착지');
              if (p != null) setState(() => _destination = p);
            },
            extra: IconButton(
              tooltip: '출발·도착 바꾸기',
              icon: const Icon(Icons.swap_vert),
              onPressed:
                  _busy ||
                      _locating ||
                      (_origin == null && _destination == null)
                  ? null
                  : _swap,
            ),
          ),
          if (widget.settings.ready) _favoriteDestinations(),
          PlanTimePicker(
            value: _time,
            arriveEnabled: _arriveAllowed,
            onChanged: _busy ? null : (t) => setState(() => _time = t),
            now: widget.now,
          ),
          const SizedBox(height: 8),
          FilledButton.icon(
            onPressed: ready ? _plan : null,
            icon: _busy
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.directions),
            label: Text(
              _busy
                  ? '탐색 중 (경유지가 있으면 30초 이상)'
                  : '${_time.label(widget.now())} 경로 찾기',
            ),
          ),
          if (_error.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 12),
              child: Text(_error),
            ),
        ],
      ),
    );
  }

  /// extra 는 검색 아이콘 앞에 붙는 버튼(출발지의 현재 위치, 도착지의 출발·도착 바꾸기). placeholder 는 비었을 때 제목.
  Widget _placeTile(
    String label,
    IconData icon,
    Place? p,
    VoidCallback onTap, {
    Widget? extra,
    String? placeholder,
  }) => ListTile(
    leading: Icon(icon),
    title: Text(p == null ? placeholder ?? '$label지 선택' : '$label: ${p.name}'),
    subtitle: p == null ? null : Text(p.address),
    trailing: extra == null
        ? const Icon(Icons.search)
        : Row(
            mainAxisSize: MainAxisSize.min,
            children: [extra, const Icon(Icons.search)],
          ),
    onTap: onTap,
  );

  Widget _favoriteDestinations() => Padding(
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Text('자주 가는 곳', style: Theme.of(context).textTheme.labelLarge),
            const Spacer(),
            TextButton.icon(
              onPressed: _busy ? null : _manageFavorites,
              icon: const Icon(Icons.edit_outlined, size: 18),
              label: const Text('추가·관리'),
            ),
          ],
        ),
        if (_favorites == null && _favoritesError.isEmpty)
          const LinearProgressIndicator()
        else if (_favorites != null && _favorites!.isEmpty)
          const Text('집·회사·자주 가는 장소를 추가해 빠르게 선택하세요.')
        else
          Wrap(
            spacing: 8,
            runSpacing: 4,
            children: [
              for (final favorite in _favorites ?? const <FavoritePlace>[])
                ActionChip(
                  avatar: Icon(_favoriteIcon(favorite.kind), size: 18),
                  label: Text(favorite.label),
                  onPressed: _busy
                      ? null
                      : () => setState(() => _destination = favorite.place),
                ),
            ],
          ),
        if (_favoritesError.isNotEmpty)
          Text(
            _favoritesError,
            style: TextStyle(color: Theme.of(context).colorScheme.error),
          ),
      ],
    ),
  );

  static IconData _favoriteIcon(FavoriteKind kind) => switch (kind) {
    FavoriteKind.home => Icons.home,
    FavoriteKind.work => Icons.business,
    FavoriteKind.custom => Icons.star,
  };

  /// 구간 i(출발→경유1 이 0)의 수단 고정 선택.
  Widget _segmentMode(int i) => Padding(
    padding: const EdgeInsets.only(left: 16, right: 16, bottom: 8),
    child: Row(
      children: [
        SizedBox(
          width: 72,
          child: Text(
            '구간 ${i + 1} 수단',
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ),
        Expanded(
          child: SegmentedButton<SegmentMode>(
            segments: [
              for (final m in SegmentMode.values)
                ButtonSegment(value: m, label: Text(m.label)),
            ],
            selected: {_modes[i]},
            showSelectedIcon: false,
            style: const ButtonStyle(
              visualDensity: VisualDensity.compact,
              padding: WidgetStatePropertyAll(
                EdgeInsets.symmetric(horizontal: 6),
              ),
            ),
            // 경로 요청 중에는 수단을 바꾸지 않는다(null 이면 버튼이 꺼진다).
            onSelectionChanged: _busy
                ? null
                : (s) => setState(() {
                    _modes[i] = s.first;
                    _fitTime();
                  }),
          ),
        ),
      ],
    ),
  );
}
