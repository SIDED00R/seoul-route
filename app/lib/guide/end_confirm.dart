import 'package:flutter/material.dart';

/// 안내 중 뒤로가기를 눌렀을 때 묻는다. "종료" 면 true, "계속 안내"·바깥 탭이면 false.
Future<bool> confirmEndGuide(BuildContext context) async {
  final end = await showDialog<bool>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: const Text('안내를 종료할까요?'),
      content: const Text('종료하면 남은 위치 샘플을 보내고, 표본이 충분한 걷기·자전거 속도는 내 속도에 반영합니다.'),
      actions: [
        TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('계속 안내')),
        FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('종료')),
      ],
    ),
  );
  return end ?? false;
}
