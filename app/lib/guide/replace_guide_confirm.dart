import 'package:flutter/material.dart';

/// 안내 중에 다른 여정으로 안내를 시작하려 할 때 묻는다. 안내는 한 번에 하나뿐이라(위치 스트림·알림창이 하나)
/// 새로 시작하려면 하던 안내를 끝내야 한다. "새로 시작" 이면 true, "계속 안내"·바깥 탭이면 false.
Future<bool> confirmReplaceGuide(BuildContext context) async {
  final replace = await showDialog<bool>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: const Text('하던 안내를 끝내고 새로 시작할까요?'),
      content: const Text('안내는 한 번에 하나만 할 수 있습니다. 하던 안내는 지금 종료되고, '
          '남은 위치 샘플을 보내 표본이 충분한 걷기·자전거 속도는 내 속도에 반영합니다.'),
      actions: [
        TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('계속 안내')),
        FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('새로 시작')),
      ],
    ),
  );
  return replace ?? false;
}
