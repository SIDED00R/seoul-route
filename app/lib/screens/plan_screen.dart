import 'package:flutter/material.dart';

import '../api/client.dart';
import '../location/current_location.dart';
import '../models/place.dart';
import '../models/plan_request.dart';
import '../settings/settings_store.dart';
import 'place_search_screen.dart';
import 'results_screen.dart';
import 'settings_screen.dart';

/// 홈: 출발·경유(최대 5)·도착을 고르고 구간마다 수단을 고정한 뒤 경로를 요청한다. 출발지는 현재 위치로도 고를 수 있다.
class PlanScreen extends StatefulWidget {
  const PlanScreen({super.key, required this.settings, this.locate = currentPlace});

  final Settings settings;

  /// 현재 위치를 Place 로 받는다. 실패하면 LocationException. 테스트가 가짜로 바꾼다.
  final Future<Place> Function() locate;

  @override
  State<PlanScreen> createState() => _PlanScreenState();
}

class _PlanScreenState extends State<PlanScreen> {
  late Settings _settings = widget.settings;
  Place? _origin;
  Place? _destination;
  final List<Place> _via = [];
  final List<SegmentMode> _modes = [SegmentMode.any];
  bool _busy = false;
  bool _locating = false;
  String _error = '';

  static const maxVia = 5;

  ApiClient get _api => ApiClient(baseUrl: _settings.baseUrl, token: _settings.token);

  Future<Place?> _pick(String title) => Navigator.push<Place>(
        context,
        MaterialPageRoute(builder: (_) => PlaceSearchScreen(api: _api, title: title)),
      );

  Future<void> _openSettings() async {
    final s = await Navigator.push<Settings>(
      context,
      MaterialPageRoute(builder: (_) => SettingsScreen(initial: _settings)),
    );
    if (s != null) setState(() => _settings = s);
  }

  Future<void> _plan() async {
    final o = _origin;
    final d = _destination;
    if (o == null || d == null) return;
    setState(() {
      _busy = true;
      _error = '';
    });
    final req = PlanRequest(origin: o, destination: d, via: List.of(_via), segmentModes: List.of(_modes));
    try {
      final res = await _api.plan(req);
      if (!mounted) return;
      await Navigator.push(
        context,
        MaterialPageRoute(builder: (_) => ResultsScreen(api: _api, request: req, result: res)),
      );
    } on ApiException catch (e) {
      setState(() => _error = e.status == 401 ? '토큰이 없거나 만료됨 — 설정에서 입력' : '실패: ${e.message}');
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
      final p = await widget.locate();
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
        _modes.add(SegmentMode.any);
      });

  void _removeVia(int i) => setState(() {
        _via.removeAt(i);
        _modes.removeAt(i + 1);
      });

  @override
  Widget build(BuildContext context) {
    final ready = _settings.ready && _origin != null && _destination != null && !_busy && !_locating;
    return Scaffold(
      appBar: AppBar(
        title: const Text('서울 길찾기'),
        actions: [IconButton(onPressed: _openSettings, icon: const Icon(Icons.settings))],
      ),
      body: ListView(
        padding: const EdgeInsets.all(12),
        children: [
          if (!_settings.ready)
            Card(
              color: Theme.of(context).colorScheme.errorContainer,
              child: ListTile(
                title: const Text('서버 주소와 토큰을 먼저 설정하세요'),
                trailing: const Icon(Icons.chevron_right),
                onTap: _openSettings,
              ),
            ),
          _placeTile('출발', Icons.trip_origin, _origin, () async {
            if (_locating || _busy) return; // 현재 위치를 받는 동안·경로 요청 중에는 출발지 검색을 열지 않는다
            final p = await _pick('출발지');
            if (p != null) setState(() => _origin = p);
          },
              extra: _locating
                  ? const Padding(
                      padding: EdgeInsets.all(12),
                      child: SizedBox(width: 24, height: 24, child: CircularProgressIndicator(strokeWidth: 2)),
                    )
                  : IconButton(
                      tooltip: '현재 위치',
                      icon: const Icon(Icons.my_location),
                      onPressed: _busy ? null : _useCurrentLocation, // 탐색 요청 중에는 출발지를 바꾸지 않는다
                    )),
          _segmentMode(0),
          for (var i = 0; i < _via.length; i++) ...[
            ListTile(
              leading: const Icon(Icons.flag),
              title: Text('경유 ${i + 1}: ${_via[i].name}'),
              subtitle: Text(_via[i].address),
              trailing: IconButton(icon: const Icon(Icons.close), onPressed: _busy ? null : () => _removeVia(i)),
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
          _placeTile('도착', Icons.place, _destination, () async {
            if (_busy) return; // 경로 요청 중에는 도착지 검색을 열지 않는다
            final p = await _pick('도착지');
            if (p != null) setState(() => _destination = p);
          }),
          const SizedBox(height: 16),
          FilledButton.icon(
            onPressed: ready ? _plan : null,
            icon: _busy
                ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
                : const Icon(Icons.directions),
            label: Text(_busy ? '탐색 중 (경유지가 있으면 30초 이상)' : '지금 출발 경로 찾기'),
          ),
          if (_error.isNotEmpty) Padding(padding: const EdgeInsets.only(top: 12), child: Text(_error)),
        ],
      ),
    );
  }

  /// extra 는 검색 아이콘 앞에 붙는 버튼(출발지의 현재 위치).
  Widget _placeTile(String label, IconData icon, Place? p, VoidCallback onTap, {Widget? extra}) => ListTile(
        leading: Icon(icon),
        title: Text(p == null ? '$label지 선택' : '$label: ${p.name}'),
        subtitle: p == null ? null : Text(p.address),
        trailing: extra == null
            ? const Icon(Icons.search)
            : Row(mainAxisSize: MainAxisSize.min, children: [extra, const Icon(Icons.search)]),
        onTap: onTap,
      );

  /// 구간 i(출발→경유1 이 0)의 수단 고정 선택.
  Widget _segmentMode(int i) => Padding(
        padding: const EdgeInsets.only(left: 16, right: 16, bottom: 8),
        child: Row(
          children: [
            SizedBox(width: 72, child: Text('구간 ${i + 1} 수단', style: Theme.of(context).textTheme.bodySmall)),
            Expanded(
              child: SegmentedButton<SegmentMode>(
                segments: [for (final m in SegmentMode.values) ButtonSegment(value: m, label: Text(m.label))],
                selected: {_modes[i]},
                showSelectedIcon: false,
                style: const ButtonStyle(
                  visualDensity: VisualDensity.compact,
                  padding: WidgetStatePropertyAll(EdgeInsets.symmetric(horizontal: 6)),
                ),
                // 경로 요청 중에는 수단을 바꾸지 않는다(null 이면 버튼이 꺼진다).
                onSelectionChanged: _busy ? null : (s) => setState(() => _modes[i] = s.first),
              ),
            ),
          ],
        ),
      );
}
