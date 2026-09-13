import 'package:flutter/material.dart';

import '../api/client.dart';
import '../settings/settings_store.dart';

/// 서버 주소와 토큰 입력. "연결 확인" 은 /health(무인증)와 /users/me·/users/me/speed(인증) 를 실제로 호출한다.
class SettingsScreen extends StatefulWidget {
  const SettingsScreen({super.key, required this.initial});

  final Settings initial;

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  late final TextEditingController _url = TextEditingController(text: widget.initial.baseUrl);
  late final TextEditingController _token = TextEditingController(text: widget.initial.token);
  String _status = '';
  bool _busy = false;

  Settings get _current => Settings(baseUrl: normalizeBaseUrl(_url.text), token: _token.text.trim());

  Future<void> _check() async {
    setState(() {
      _busy = true;
      _status = '확인 중…';
    });
    final s = _current;
    final api = ApiClient(baseUrl: s.baseUrl, token: s.token);
    // 응답 전에 화면을 나가면 State 가 폐기되므로 await 뒤 setState 마다 mounted 를 본다.
    try {
      final h = await api.health();
      final me = await api.me();
      final sp = await api.mySpeed();
      if (!mounted) return;
      setState(() => _status = '서버 OK (db ${h['db']}, otp ${h['otp']}) · 사용자 ${me['user_id']}\n'
          '내 속도 — 걷기 ${sp['walk']?.label} · 자전거 ${sp['bicycle']?.label}');
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _status = '실패: $e');
    } catch (e) {
      if (!mounted) return;
      setState(() => _status = '연결 실패: $e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _save() async {
    await SettingsStore().save(_current);
    if (!mounted) return;
    Navigator.pop(context, _current);
  }

  @override
  void dispose() {
    _url.dispose();
    _token.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('설정')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          TextField(
            controller: _url,
            decoration: const InputDecoration(
              labelText: '서버 주소',
              helperText: '에뮬레이터: http://10.0.2.2:8081 · 실기기: http://<PC 내부 IP>:8081',
            ),
            keyboardType: TextInputType.url,
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _token,
            decoration: const InputDecoration(
              labelText: '토큰(JWT)',
              helperText: 'PC 에서 go run ./cmd/devtoken <이름> 으로 발급',
            ),
            maxLines: 3,
          ),
          const SizedBox(height: 16),
          Row(
            children: [
              OutlinedButton(onPressed: _busy ? null : _check, child: const Text('연결 확인')),
              const SizedBox(width: 12),
              FilledButton(onPressed: _busy ? null : _save, child: const Text('저장')),
            ],
          ),
          const SizedBox(height: 12),
          Text(_status),
        ],
      ),
    );
  }
}
