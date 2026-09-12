import 'package:flutter/material.dart';

import '../api/client.dart';
import '../models/place.dart';
import '../models/plan_request.dart';
import '../settings/settings_store.dart';
import 'place_search_screen.dart';
import 'results_screen.dart';
import 'settings_screen.dart';

/// 홈: 출발·경유(최대 5)·도착을 고르고 구간마다 수단을 고정한 뒤 경로를 요청한다.
class PlanScreen extends StatefulWidget {
  const PlanScreen({super.key, required this.settings});

  final Settings settings;

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
    final ready = _settings.ready && _origin != null && _destination != null && !_busy;
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
            final p = await _pick('출발지');
            if (p != null) setState(() => _origin = p);
          }),
          _segmentMode(0),
          for (var i = 0; i < _via.length; i++) ...[
            ListTile(
              leading: const Icon(Icons.flag),
              title: Text('경유 ${i + 1}: ${_via[i].name}'),
              subtitle: Text(_via[i].address),
              trailing: IconButton(icon: const Icon(Icons.close), onPressed: () => _removeVia(i)),
            ),
            _segmentMode(i + 1),
          ],
          if (_via.length < maxVia)
            TextButton.icon(
              onPressed: () async {
                final p = await _pick('경유지 ${_via.length + 1}');
                if (p != null) _addVia(p);
              },
              icon: const Icon(Icons.add),
              label: const Text('경유지 추가'),
            ),
          _placeTile('도착', Icons.place, _destination, () async {
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

  Widget _placeTile(String label, IconData icon, Place? p, VoidCallback onTap) => ListTile(
        leading: Icon(icon),
        title: Text(p == null ? '$label지 선택' : '$label: ${p.name}'),
        subtitle: p == null ? null : Text(p.address),
        trailing: const Icon(Icons.search),
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
                onSelectionChanged: (s) => setState(() => _modes[i] = s.first),
              ),
            ),
          ],
        ),
      );
}
