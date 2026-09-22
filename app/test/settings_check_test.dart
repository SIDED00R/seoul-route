import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:seoul_route/api/client.dart';
import 'package:seoul_route/screens/settings_screen.dart';
import 'package:seoul_route/settings/settings_store.dart';

import 'scroll_to.dart';
import 'settings_test.dart' show MemoryTokenStorage;

// 설정 화면 "연결 확인"이 실패 원인을 구분해 보여 준다(주소 비움 / 서버 미도달 / 401). 허용목록 밖 계정 403 은
// 토큰 발급 전 /auth/google 에서만 나므로 Google 로그인 경로에서 알린다.
// 오늘(2026-09-22) Dev 앱이 에뮬레이터용 기본 주소로 조용히 타임아웃만 내던 것을 화면에서 바로 알게 하기 위함.
void main() {
  // http.runWithClient 존 안에서 pump·tap 을 해야 화면의 top-level http 호출이 MockClient 로 간다.
  Future<void> pump(WidgetTester tester, String baseUrl) async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    await tester.pumpWidget(MaterialApp(
      home: SettingsScreen(
        initial: Settings(baseUrl: baseUrl, token: 'tok'),
        settingsStore: SettingsStore(tokenStorage: MemoryTokenStorage()),
      ),
    ));
  }

  Future<String> check(WidgetTester tester) async {
    await scrollTo(tester, find.text('연결 확인'));
    await tester.tap(find.text('연결 확인'));
    await tester.pumpAndSettle();
    final texts = find.byType(Text).evaluate().map((e) => (e.widget as Text).data ?? '');
    // 상태 문구만 고른다("서버 주소" 라벨은 제외).
    const prefixes = ['서버 주소를', '서버에 닿지', '서버는 닿았', '서버 OK', '실패'];
    return texts.firstWhere((t) => prefixes.any(t.startsWith), orElse: () => '');
  }

  testWidgets('서버 주소가 비어 있으면 요청 없이 입력을 안내한다', (tester) async {
    await pump(tester, '');
    expect(await check(tester), '서버 주소를 먼저 입력하세요');
  });

  testWidgets('서버에 닿지 않으면 주소와 확인할 것을 알려 준다', (tester) async {
    await http.runWithClient(() async {
      await pump(tester, 'http://10.0.2.2:8082');
      final status = await check(tester);
      expect(status, startsWith('서버에 닿지 않음: http://10.0.2.2:8082'));
    }, () => MockClient((_) async => throw const SocketException('connection refused')));
  });

  testWidgets('허용목록 밖 계정의 Google 로그인 403 은 그 사실을 알려 준다', (tester) async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    await tester.pumpWidget(MaterialApp(
      home: SettingsScreen(
        initial: const Settings(baseUrl: 'http://x', token: ''),
        settingsStore: SettingsStore(tokenStorage: MemoryTokenStorage()),
        // 서버 /auth/google 이 허용목록 밖 계정에 주는 응답(auth_handler.go)을 그대로 흉내 낸다.
        login: (_) async => throw const ApiException(403, '이 서버에서 허용되지 않은 계정'),
      ),
    ));
    await tester.tap(find.text('Google 계정으로 로그인'));
    await tester.pumpAndSettle();
    await scrollTo(tester, find.textContaining('허용목록에 없습니다')); // 상태 문구는 목록 맨 아래
  });

  testWidgets('401 은 토큰 문제로 안내하고, 성공은 서버 버전을 보여 준다', (tester) async {
    Future<String> withStatus(int meStatus, {String health = '{"db":"ok","otp":"ok","version":"abc1234"}'}) async {
      return http.runWithClient(() async {
        await pump(tester, 'http://x');
        return check(tester);
      }, () => MockClient((req) async {
        if (req.url.path == '/health') return http.Response(health, 200);
        if (meStatus != 200) return http.Response('{"error":"e"}', meStatus);
        if (req.url.path == '/users/me') return http.Response('{"user_id":"u1"}', 200);
        return http.Response('{}', 200);
      }));
    }

    expect(await withStatus(401), startsWith('서버는 닿았지만 토큰이 없거나 만료됨'));
    expect(await withStatus(200), contains('버전 abc1234'));
  });
}
