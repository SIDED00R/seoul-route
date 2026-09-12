import 'package:flutter/material.dart';

import '../api/client.dart';
import '../models/itinerary.dart';
import '../models/plan_request.dart';
import '../widgets/mode_icon.dart';
import 'detail_screen.dart';

/// 경로 후보 목록. 서버가 최저시간순으로 정렬해 준 순서를 그대로 보여준다.
class ResultsScreen extends StatelessWidget {
  const ResultsScreen({super.key, required this.api, required this.request, required this.result});

  final ApiClient api;
  final PlanRequest request;
  final PlanResult result;

  @override
  Widget build(BuildContext context) {
    final its = result.itineraries;
    return Scaffold(
      appBar: AppBar(title: Text('${request.origin.name} → ${request.destination.name}')),
      body: its.isEmpty
          ? Center(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Text(result.reason ?? '경로 없음', textAlign: TextAlign.center),
              ),
            )
          : ListView.separated(
              itemCount: its.length + 1,
              separatorBuilder: (_, _) => const Divider(height: 1),
              itemBuilder: (context, i) {
                if (i == its.length) {
                  return Padding(
                    padding: const EdgeInsets.all(12),
                    child: Text(result.note ?? '', style: Theme.of(context).textTheme.bodySmall),
                  );
                }
                final it = its[i];
                return ListTile(
                  leading: CircleAvatar(child: Text('${i + 1}')),
                  title: Text('${it.minutes}분 · 환승 ${it.transfers}회 · 도보 ${(it.walkM / 1000).toStringAsFixed(1)}km'),
                  subtitle: Wrap(
                    spacing: 4,
                    runSpacing: 4,
                    children: [
                      for (final leg in it.legs)
                        Chip(
                          avatar: Icon(modeIcon(leg), size: 16, color: modeColor(leg)),
                          label: Text('${leg.label} ${(leg.durationSec / 60).round()}분'),
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
            ),
    );
  }
}
