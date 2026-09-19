import 'package:flutter/material.dart';

import 'screens/home_screen.dart';
import 'settings/settings_store.dart';

// 서울 길찾기 앱. 화면은 screens/, 서버 호출은 api/, 응답 모델은 models/ 에 둔다.
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final settings = await SettingsStore().load();
  runApp(SeoulRouteApp(settings: settings));
}

class SeoulRouteApp extends StatelessWidget {
  const SeoulRouteApp({super.key, required this.settings});

  final Settings settings;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: '서울 길찾기',
      theme: ThemeData(colorSchemeSeed: Colors.teal, useMaterial3: true),
      home: HomeScreen(settings: settings),
    );
  }
}
