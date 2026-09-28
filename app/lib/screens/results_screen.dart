import 'package:flutter/material.dart';

import '../api/client.dart';
import '../models/itinerary.dart';
import '../models/plan_request.dart';
import '../models/result_sort.dart';
import '../widgets/mode_icon.dart';
import '../util/stay_label.dart';
import 'detail_screen.dart';

/// 경로 후보 목록. 처음에는 서버 순서(추천순)로 보여 주고, 위의 전환으로 최소시간순으로 바꿔 볼 수 있다.
class ResultsScreen extends StatefulWidget {
  const ResultsScreen({super.key, required this.api, required this.request, required this.result});

  final ApiClient api;
  final PlanRequest request;
  final PlanResult result;

  @override
  State<ResultsScreen> createState() => _ResultsScreenState();
}

class _ResultsScreenState extends State<ResultsScreen> {
  ResultSort _sort = ResultSort.recommended;

  @override
  Widget build(BuildContext context) {
    final request = widget.request;
    final result = widget.result;
    final its = sortItineraries(result.itineraries, _sort);
    return Scaffold(
      appBar: AppBar(title: Text('${request.origin.name} → ${request.destination.name}')),
      body: its.isEmpty
          ? Center(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Text(result.reason ?? '경로 없음', textAlign: TextAlign.center),
              ),
            )
          : Column(
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
                  child: SegmentedButton<ResultSort>(
                    segments: [
                      for (final s in ResultSort.values) ButtonSegment(value: s, label: Text(s.label)),
                    ],
                    selected: {_sort},
                    showSelectedIcon: false,
                    style: const ButtonStyle(visualDensity: VisualDensity.compact),
                    onSelectionChanged: (s) => setState(() => _sort = s.first),
                  ),
                ),
                if (request.totalStayMin > 0)
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 12),
                    child: Text('소요 시간에 경유지 체류 ${stayLabel(request.totalStayMin)}이 들어 있습니다',
                        style: Theme.of(context).textTheme.bodySmall),
                  ),
                Expanded(child: _list(its)),
              ],
            ),
    );
  }

  Widget _list(List<Itinerary> its) {
    final api = widget.api;
    final request = widget.request;
    return ListView.separated(
      itemCount: its.length + 1,
      separatorBuilder: (_, _) => const Divider(height: 1),
      itemBuilder: (context, i) {
        if (i == its.length) {
          return Padding(
            padding: const EdgeInsets.all(12),
            child: Text(widget.result.note ?? '', style: Theme.of(context).textTheme.bodySmall),
          );
        }
        final it = its[i];
        return ListTile(
          leading: CircleAvatar(child: Text('${i + 1}')),
          title: Text('${it.minutes}분 · 환승 ${it.transfers}회 · '
              '도보 ${(it.walkM / 1000).toStringAsFixed(1)}km'),
          subtitle: Wrap(
            spacing: 4,
            runSpacing: 4,
            children: [
              // 출발 대기·실시간 배지는 수단 칩 앞에 둔다(제목 줄이 꺾이지 않게).
              for (final label in [it.departLabel, it.realtimeLabel, it.crossingLabel, it.replannedLabel])
                if (label != null)
                  Chip(
                    label: Text(label, style: const TextStyle(fontSize: 12)),
                    backgroundColor: Colors.teal.shade50,
                    padding: EdgeInsets.zero,
                    visualDensity: VisualDensity.compact,
                  ),
              // 노선 색이 있으면 칩을 그 색으로 칠한다(2호선 초록처럼). 없으면 기본 흰 칩에 수단 색 아이콘.
              for (final leg in it.legs)
                Chip(
                  avatar: Icon(modeIcon(leg), size: 16,
                      color: leg.color.isEmpty ? modeColor(leg) : legTextColor(leg)),
                  label: Text('${leg.label} ${(leg.durationSec / 60).round()}분',
                      style: leg.color.isEmpty ? null : TextStyle(color: legTextColor(leg))),
                  backgroundColor: leg.color.isEmpty ? null : modeColor(leg),
                  padding: EdgeInsets.zero,
                  visualDensity: VisualDensity.compact,
                ),
            ],
          ),
          onTap: () => Navigator.push(
            context,
            MaterialPageRoute(
              builder: (_) => DetailScreen(api: api, request: request, itinerary: it, index: i + 1),
            ),
          ),
        );
      },
    );
  }
}
