import 'dart:async';

import 'package:flutter/material.dart';

import '../api/client.dart';
import '../guide/active_guide.dart';
import '../guide/guide_restore.dart';
import '../location/current_location.dart';
import '../models/place.dart';
import '../settings/settings_store.dart';
import '../util/app_task.dart';
import 'current_guide_tab.dart';
import 'plan_screen.dart';
import 'recent_routes_tab.dart';
import 'settings_screen.dart';

/// 홈. 최근 경로·현재 경로·길찾기 세 탭이다. 안내는 화면이 아니라 세션이 들고 있어(ActiveGuide) 안내 중에도
/// 길찾기 탭에서 다른 경로를 찾아볼 수 있고, 현재 경로 탭으로 안내 화면에 돌아간다.
class HomeScreen extends StatefulWidget {
  const HomeScreen({
    super.key,
    required this.settings,
    this.locate = currentPlace,
    this.settingsStore = const SettingsStore(),
    this.initialTab = 2,
    this.restore = restoreGuide,
  });

  final Settings settings;
  final SettingsStore settingsStore;

  /// 현재 위치를 Place 로 받는다. 실패하면 LocationException. 테스트가 가짜로 바꾼다.
  final Future<Place> Function(ApiClient api) locate;

  /// 처음 보여 줄 탭(0 최근 · 1 현재 · 2 길찾기). 기본은 길찾기다.
  final int initialTab;

  /// 디스크에 남은 안내를 되살린다. 테스트가 가짜로 바꾼다.
  final Future<bool> Function(ApiClient api) restore;

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  late Settings _settings = widget.settings;
  late int _tab = widget.initialTab;

  @override
  void initState() {
    super.initState();
    // 앱이 죽어 있는 동안에도 안내는 끝나지 않는다 — 남아 있으면 이어받는다.
    if (_settings.ready) unawaited(widget.restore(_api));
  }

  /// 최근 경로에서 고른 요청. 길찾기 탭이 이 값으로 입력을 채운다.
  PlanScreenPreset? _preset;

  ApiClient get _api => ApiClient(baseUrl: _settings.baseUrl, token: _settings.token);

  Future<void> _openSettings() async {
    final s = await Navigator.push<Settings>(
      context,
      MaterialPageRoute(builder: (_) => SettingsScreen(initial: _settings, settingsStore: widget.settingsStore)),
    );
    if (s != null) setState(() => _settings = s);
  }

  /// 최근 경로 탭에 들어온 횟수. 들어올 때마다 목록을 새로 받게 하는 신호다.
  int _recentVisits = 0;

  /// 최근 경로를 골랐을 때: 길찾기 탭에 그 출발·도착을 채우고 바로 탐색한다.
  void _useRecent(PlanScreenPreset p) => setState(() {
        _preset = p;
        _tab = 2;
      });

  void _selectTab(int i) => setState(() {
        _tab = i;
        if (i == 0) _recentVisits++;
      });

  @override
  Widget build(BuildContext context) {
    final api = _api;
    return ValueListenableBuilder(
      valueListenable: ActiveGuide.instance.session,
      builder: (context, session, child) => PopScope(
        // 안내 중에는 뒤로가기로 앱을 끝내지 않는다 — 액티비티가 끝나면 안내가 통째로 사라진다. 대신 뒤로 보낸다.
        canPop: session == null || session.ended,
        onPopInvokedWithResult: (didPop, _) {
          if (!didPop) unawaited(moveAppToBack());
        },
        child: child!,
      ),
      child: _home(api),
    );
  }

  Widget _home(ApiClient api) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('서울 길찾기'),
        actions: [IconButton(onPressed: _openSettings, icon: const Icon(Icons.settings))],
      ),
      // 탭을 바꿔도 길찾기 탭에 고른 출발·도착이 남아 있어야 하므로 화면을 버리지 않고 쌓아 둔다.
      body: IndexedStack(
        index: _tab,
        children: [
          RecentRoutesTab(api: api, ready: _settings.ready, onPick: _useRecent, refreshKey: _recentVisits),
          CurrentGuideTab(onEnded: () => setState(() {})),
          PlanScreen(
            settings: _settings,
            locate: widget.locate,
            settingsStore: widget.settingsStore,
            preset: _preset,
            onOpenSettings: _openSettings,
          ),
        ],
      ),
      bottomNavigationBar: ValueListenableBuilder(
        valueListenable: ActiveGuide.instance.session,
        builder: (context, session, _) => NavigationBar(
          selectedIndex: _tab,
          onDestinationSelected: _selectTab,
          destinations: [
            const NavigationDestination(icon: Icon(Icons.history), label: '최근 경로'),
            NavigationDestination(
              // 안내 중이면 탭에 점을 찍어 다른 탭에 있어도 진행 중임을 알린다.
              icon: Badge(isLabelVisible: session != null && !session.ended, child: const Icon(Icons.navigation)),
              label: '현재 경로',
            ),
            const NavigationDestination(icon: Icon(Icons.directions), label: '길찾기'),
          ],
        ),
      ),
    );
  }
}
