import 'package:flutter/material.dart';

import '../models/plan_time.dart';

/// 길찾기 시각 조건 고르기. 출발·도착 시각을 고르면 기본값(출발 = 지금, 도착 = 1시간 뒤, 5분 단위 올림)이 들어가고,
/// 시각 버튼을 누르면 날짜(오늘부터 30일)와 시각을 묻는다. arriveEnabled 가 false 면 도착 시각을 고를 수 없다.
class PlanTimePicker extends StatelessWidget {
  const PlanTimePicker({
    super.key,
    required this.value,
    required this.onChanged,
    this.arriveEnabled = true,
    this.now = DateTime.now,
  });

  final PlanTime value;
  final ValueChanged<PlanTime>? onChanged; // null 이면 꺼진다
  final bool arriveEnabled;
  final DateTime Function() now;

  DateTime _default(PlanTimeKind kind) {
    final t = now().add(
      kind == PlanTimeKind.arrive ? const Duration(hours: 1) : Duration.zero,
    );
    // 초 이하가 남아 있으면 다음 분부터 5분 단위로 올린다(10:05:30 → 10:10, 10:05:00 → 10:05).
    final whole = t.second == 0 && t.millisecond == 0 && t.microsecond == 0;
    return DateTime(t.year, t.month, t.day, t.hour, ((t.minute + (whole ? 0 : 1)) / 5).ceil() * 5);
  }

  Future<void> _pickAt(BuildContext context) async {
    final base = value.at ?? _default(value.kind);
    final today = now();
    final first = DateTime(today.year, today.month, today.day);
    final day = await showDatePicker(
      context: context,
      initialDate: base.isBefore(first) ? first : base,
      firstDate: first,
      lastDate: first.add(const Duration(days: 30)),
    );
    if (day == null || !context.mounted) return;
    final tod = await showTimePicker(
      context: context,
      initialTime: TimeOfDay.fromDateTime(base),
    );
    if (tod == null) return;
    onChanged?.call(
      PlanTime(
        value.kind,
        DateTime(day.year, day.month, day.day, tod.hour, tod.minute),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final change = onChanged;
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SegmentedButton<PlanTimeKind>(
            segments: [
              for (final k in PlanTimeKind.values)
                ButtonSegment(
                  value: k,
                  label: Text(k.label),
                  enabled: k != PlanTimeKind.arrive || arriveEnabled,
                ),
            ],
            selected: {value.kind},
            showSelectedIcon: false,
            style: const ButtonStyle(visualDensity: VisualDensity.compact),
            onSelectionChanged: change == null
                ? null
                : (s) => change(
                    s.first == PlanTimeKind.now
                        ? PlanTime.now
                        : PlanTime(s.first, _default(s.first)),
                  ),
          ),
          if (value.kind != PlanTimeKind.now)
            TextButton.icon(
              onPressed: change == null ? null : () => _pickAt(context),
              icon: const Icon(Icons.schedule),
              label: Text(value.label(now())),
            ),
          if (!arriveEnabled)
            Text(
              '경유지가 있거나 구간 수단을 고정하면 도착 시각은 고를 수 없습니다',
              style: Theme.of(context).textTheme.bodySmall,
            ),
        ],
      ),
    );
  }
}
