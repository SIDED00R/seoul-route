import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../util/stay_label.dart';

/// 경유지에서 머무는 시간 선택. 자주 쓰는 값은 목록에서 한 번에 고르고, 그 밖의 값은 "직접 입력"으로 분 단위로 넣는다.
class ViaStayPicker extends StatelessWidget {
  const ViaStayPicker({super.key, required this.minutes, required this.onChanged});

  final int minutes;
  final ValueChanged<int>? onChanged; // null 이면 바꿀 수 없다(경로 요청 중)

  static const presets = [0, 5, 10, 15, 20, 30, 45, 60, 90, 120];
  static const maxMinutes = 180; // 서버 route.MaxStayMin 과 같다
  static const _custom = -1;

  @override
  Widget build(BuildContext context) {
    final values = {...presets, minutes}.toList()..sort();
    final change = onChanged;
    return Row(
      children: [
        const Icon(Icons.schedule, size: 18),
        const SizedBox(width: 8),
        Text('머무는 시간', style: Theme.of(context).textTheme.bodySmall),
        const Spacer(),
        DropdownButton<int>(
          value: minutes,
          items: [
            for (final m in values) DropdownMenuItem(value: m, child: Text(stayLabel(m))),
            const DropdownMenuItem(value: _custom, child: Text('직접 입력…')),
          ],
          onChanged: change == null
              ? null
              : (v) async {
                  if (v == null) return;
                  if (v != _custom) return change(v);
                  final m = await askStayMinutes(context, minutes);
                  if (m != null) change(m);
                },
        ),
      ],
    );
  }
}

/// 체류 분을 직접 입력받는다(1~maxMinutes). 취소하면 null.
Future<int?> askStayMinutes(BuildContext context, int initial) {
  final text = TextEditingController(text: initial > 0 ? '$initial' : '');
  return showDialog<int>(
    context: context,
    builder: (context) => StatefulBuilder(
      builder: (context, setState) {
        final v = int.tryParse(text.text);
        final valid = v != null && v >= 1 && v <= ViaStayPicker.maxMinutes;
        return AlertDialog(
          title: const Text('머무는 시간'),
          content: TextField(
            controller: text,
            autofocus: true,
            keyboardType: TextInputType.number,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: InputDecoration(
              suffixText: '분',
              helperText: '1~${ViaStayPicker.maxMinutes}분',
              errorText: text.text.isEmpty || valid ? null : '1~${ViaStayPicker.maxMinutes}분 사이로 넣어 주세요',
            ),
            onChanged: (_) => setState(() {}),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(context), child: const Text('취소')),
            FilledButton(onPressed: valid ? () => Navigator.pop(context, v) : null, child: const Text('확인')),
          ],
        );
      },
    ),
  );
}
