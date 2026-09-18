import 'package:flutter/material.dart';

/// 안내 화면 맨 위 카드: 도착 예정과 남은 시간, 지금 할 일, 다음 할 일.
class GuideCard extends StatelessWidget {
  const GuideCard({
    super.key,
    required this.icon,
    required this.color,
    required this.eta,
    required this.remainMin,
    required this.now,
    required this.next,
  });

  final IconData icon;
  final Color color;
  final DateTime? eta; // 계획 기준 도착 시각. 없으면 남은 시간만 보여 준다
  final int remainMin;
  final String now;
  final String next;

  static String hhmm(DateTime t) =>
      '${t.hour.toString().padLeft(2, '0')}:${t.minute.toString().padLeft(2, '0')}';

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final at = eta;
    return Card(
      margin: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(icon, color: color, size: 32),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    '${at == null ? '' : '도착 예정 ${hhmm(at.toLocal())} · '}남은 $remainMin분',
                    style: theme.textTheme.bodySmall,
                  ),
                  const SizedBox(height: 2),
                  Text(now, style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold)),
                  const SizedBox(height: 2),
                  Text('다음: $next', style: theme.textTheme.bodySmall),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
