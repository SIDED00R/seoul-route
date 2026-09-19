import 'package:flutter/material.dart';

import '../guide/active_guide.dart';
import '../guide/guide_session.dart';
import '../util/hhmm.dart';
import '../util/leg_names.dart';
import '../widgets/mode_icon.dart';
import 'guide_screen.dart';

/// 홈의 "현재 경로" 탭. 진행 중인 안내를 요약해 보여 주고 안내 화면으로 돌아가게 한다. 안내 중이 아니면 비어 있다.
class CurrentGuideTab extends StatelessWidget {
  const CurrentGuideTab({super.key, this.onEnded});

  /// 안내가 끝나 탭이 비었을 때 홈에 알린다(탭 배지를 지우려고).
  final VoidCallback? onEnded;

  @override
  Widget build(BuildContext context) {
    return ValueListenableBuilder(
      valueListenable: ActiveGuide.instance.session,
      builder: (context, session, _) {
        if (session == null || session.ended) {
          return const Center(
            child: Padding(
              padding: EdgeInsets.all(24),
              child: Text('안내 중인 경로가 없습니다.\n길찾기 탭에서 경로를 찾아 안내를 시작하세요.', textAlign: TextAlign.center),
            ),
          );
        }
        return _Summary(session: session, onEnded: onEnded);
      },
    );
  }
}

class _Summary extends StatefulWidget {
  const _Summary({required this.session, this.onEnded});

  final GuideSession session;
  final VoidCallback? onEnded;

  @override
  State<_Summary> createState() => _SummaryState();
}

class _SummaryState extends State<_Summary> {
  @override
  void initState() {
    super.initState();
    widget.session.addListener(_changed);
  }

  /// 안내를 갈아타면 이 상태는 그대로 남고 새 세션만 전달된다. 구독을 옮긴다.
  @override
  void didUpdateWidget(_Summary old) {
    super.didUpdateWidget(old);
    if (identical(old.session, widget.session)) return;
    old.session.removeListener(_changed);
    widget.session.addListener(_changed);
  }

  @override
  void dispose() {
    widget.session.removeListener(_changed);
    super.dispose();
  }

  void _changed() {
    if (!mounted) return;
    setState(() {});
    if (widget.session.ended) widget.onEnded?.call();
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.session;
    final leg = s.tracker.current;
    final at = s.eta;
    return ListView(
      padding: const EdgeInsets.all(12),
      children: [
        Card(
          child: ListTile(
            leading: Icon(modeIcon(leg), color: modeColor(leg)),
            title: Text(s.instr.now, style: const TextStyle(fontWeight: FontWeight.bold)),
            subtitle: Text('${at == null ? '' : '도착 예정 ${hhmm(at.toLocal())} · '}남은 ${s.remainMin}분\n'
                '다음: ${s.instr.next}'),
            isThreeLine: true,
          ),
        ),
        ListTile(
          leading: const Icon(Icons.route),
          title: Text('${legEndpointName(s.request, s.itinerary.legs.first.fromName,
              s.itinerary.legs.first.fromLat, s.itinerary.legs.first.fromLon)} → '
              '${legEndpointName(s.request, s.itinerary.legs.last.toName,
              s.itinerary.legs.last.toLat, s.itinerary.legs.last.toLon)}'),
          subtitle: Text('구간 ${s.tracker.index + 1}/${s.itinerary.legs.length} · '
              '${hhmm(s.startedAt.toLocal())} 시작 · ${s.status}'),
        ),
        const SizedBox(height: 8),
        FilledButton.icon(
          onPressed: () => Navigator.push(
            context,
            MaterialPageRoute(builder: (_) => GuideScreen.resume(s)),
          ),
          icon: const Icon(Icons.navigation),
          label: const Text('안내 화면으로'),
        ),
      ],
    );
  }
}
