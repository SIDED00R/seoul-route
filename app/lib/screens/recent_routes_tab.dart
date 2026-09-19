import 'package:flutter/material.dart';

import '../api/client.dart';
import '../models/recent_route.dart';
import 'plan_screen.dart';

/// 홈의 "최근 경로" 탭. 계정에 쌓인 지난 검색을 새 것부터 보여 주고, 고르면 길찾기 탭에 그 출발·도착을 채운다.
class RecentRoutesTab extends StatefulWidget {
  const RecentRoutesTab(
      {super.key, required this.api, required this.ready, required this.onPick, this.refreshKey = 0});

  final ApiClient api;

  /// 서버 주소·토큰이 설정돼 있는지. 아니면 불러오지 않고 안내만 보여 준다.
  final bool ready;

  /// 홈이 이 탭에 들어올 때마다 올린다. 값이 바뀌면 목록을 새로 받는다 — 길찾기 탭에서 방금 찾은 경로가 바로 보이게.
  final int refreshKey;

  final void Function(PlanScreenPreset) onPick;

  @override
  State<RecentRoutesTab> createState() => _RecentRoutesTabState();
}

class _RecentRoutesTabState extends State<RecentRoutesTab> {
  List<RecentRoute>? _routes;
  String _error = '';
  bool _busy = false;

  @override
  void initState() {
    super.initState();
    if (widget.ready) _load();
  }

  @override
  void didUpdateWidget(RecentRoutesTab old) {
    super.didUpdateWidget(old);
    if (!widget.ready) return;
    // 계정이나 서버가 바뀌면 남아 있는 목록은 남의 기록이다. 먼저 비우고 새로 받는다.
    if (widget.api.token != old.api.token || widget.api.baseUrl != old.api.baseUrl) {
      setState(() => _routes = null);
      _load();
      return;
    }
    // 설정을 채운 직후, 그리고 이 탭에 들어올 때마다(길찾기 탭에서 방금 찾은 경로가 보이게) 새로 받는다.
    if (!old.ready || widget.refreshKey != old.refreshKey) _load();
  }

  /// 조회 세대. 요청이 겹치면 마지막 것만 화면에 반영한다 — 계정이 바뀌는 순간 앞선 요청이 늦게 돌아와 이전 계정의
  /// 기록을 다시 그리면 안 된다. 겹친다고 새 요청을 버리면 바뀐 계정을 아예 못 받는다.
  int _loadGen = 0;

  Future<void> _load() async {
    final gen = ++_loadGen;
    final api = widget.api;
    setState(() {
      _busy = true;
      _error = '';
    });
    try {
      final rs = await api.recentRoutes();
      if (mounted && gen == _loadGen) setState(() => _routes = rs);
    } on ApiException catch (e) {
      if (mounted && gen == _loadGen) {
        setState(() => _error = e.status == 401 ? '토큰이 없거나 만료됨 — 설정에서 입력' : '실패: ${e.message}');
      }
    } catch (e) {
      if (mounted && gen == _loadGen) setState(() => _error = '연결 실패: $e');
    } finally {
      if (mounted && gen == _loadGen) setState(() => _busy = false);
    }
  }

  Future<void> _clear() async {
    final yes = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('최근 경로를 모두 지울까요?'),
        content: const Text('계정에 저장된 지난 검색 기록이 사라집니다. 되돌릴 수 없습니다.'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('취소')),
          FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('모두 지우기')),
        ],
      ),
    );
    if (yes != true) return;
    // 세대를 올리면 진행 중인 조회의 결과가 버려진다. 삭제를 기다리는 동안 시작된 조회도 버려야 해서 끝난 뒤
    // 한 번 더 올린다. 버려진 조회는 _busy 를 못 내리니 여기서 내린다.
    _loadGen++;
    setState(() => _busy = false);
    try {
      await widget.api.clearRecentRoutes();
      _loadGen++;
      if (mounted) {
        setState(() {
          _routes = [];
          _busy = false;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _error = '삭제 실패: $e');
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!widget.ready) {
      return const Center(
        child: Padding(padding: EdgeInsets.all(24), child: Text('서버 주소와 토큰을 먼저 설정하세요.')),
      );
    }
    final rs = _routes;
    return RefreshIndicator(
      onRefresh: _load,
      child: ListView(
        padding: const EdgeInsets.symmetric(vertical: 8),
        children: [
          if (_busy && rs == null) const Center(child: Padding(padding: EdgeInsets.all(24), child: CircularProgressIndicator())),
          if (_error.isNotEmpty)
            Padding(padding: const EdgeInsets.all(16), child: Text(_error)),
          if (rs != null && rs.isEmpty && _error.isEmpty)
            const Padding(
              padding: EdgeInsets.all(24),
              child: Text('아직 찾아본 경로가 없습니다.\n길찾기 탭에서 경로를 찾으면 여기에 쌓입니다.', textAlign: TextAlign.center),
            ),
          for (final r in rs ?? const <RecentRoute>[])
            ListTile(
              leading: const Icon(Icons.history),
              title: Text(r.label),
              subtitle: Text([if (r.note.isNotEmpty) r.note, if (r.searchedAt != null) _when(r.searchedAt!)].join(' · ')),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => widget.onPick(PlanScreenPreset(r.request)),
            ),
          if (rs != null && rs.isNotEmpty)
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 24),
              child: OutlinedButton.icon(
                onPressed: _clear,
                icon: const Icon(Icons.delete_outline),
                label: const Text('최근 경로 모두 지우기'),
              ),
            ),
        ],
      ),
    );
  }

  /// "방금 · 12분 전 · 3시간 전 · 09-17". 정확한 시각은 필요 없고 얼마나 지났는지만 본다.
  static String _when(DateTime t) {
    final d = DateTime.now().difference(t);
    if (d.inMinutes < 1) return '방금';
    if (d.inMinutes < 60) return '${d.inMinutes}분 전';
    if (d.inHours < 24) return '${d.inHours}시간 전';
    final l = t.toLocal();
    return '${l.month.toString().padLeft(2, '0')}-${l.day.toString().padLeft(2, '0')}';
  }
}
