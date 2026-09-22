import 'package:flutter/material.dart';

import '../api/client.dart';
import '../auth/google_login.dart';
import '../env.dart';
import '../settings/settings_store.dart';
import '../guide/active_guide.dart';
import '../guide/guide_overlay.dart';

/// 서버 주소와 토큰 입력. "연결 확인" 은 /health(무인증)와 /users/me·/users/me/speed(인증) 를 실제로 호출한다.
/// prod flavor 는 서버 주소가 빌드에 고정돼 읽기 전용이고 개발용 토큰 칸이 없다(Env).
class SettingsScreen extends StatefulWidget {
  const SettingsScreen({
    super.key,
    required this.initial,
    this.login = _googleLogin,
    this.settingsStore = const SettingsStore(),
  });

  final Settings initial;
  final SettingsStore settingsStore;
  // Google 로그인 실행. 기본은 GoogleLogin(계정 선택창이 시스템 UI 라 테스트에서 바꿔 끼운다).
  final Future<LoginResult?> Function(ApiClient api) login;

  static Future<LoginResult?> _googleLogin(ApiClient api) =>
      GoogleLogin(api).signIn();

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  late final TextEditingController _url = TextEditingController(
    text: widget.initial.baseUrl,
  );
  late final TextEditingController _token = TextEditingController(
    text: widget.initial.token,
  );
  String _status = '';
  bool _busy = false;
  late bool _voiceGuide = widget.initial.voiceGuide;
  late bool _overlayGuide = widget.initial.overlayGuide;
  late double _overlayOpacity = widget.initial.overlayOpacity;

  @override
  void initState() {
    super.initState();
    // 안내 중 미니 지도 손잡이로 바꾼 값은 저장소에만 있고 initial(앱 시작 때 읽은 값)에는 없다.
    SettingsStore.loadOverlayOpacity().then((v) {
      if (mounted) setState(() => _overlayOpacity = v);
    });
  }

  Settings get _current => Settings(
    baseUrl: normalizeBaseUrl(_url.text),
    token: _token.text.trim(),
    voiceGuide: _voiceGuide,
    overlayGuide: _overlayGuide,
    overlayOpacity: _overlayOpacity,
  );

  /// 켜기·끄기 모두 "저장" 때 진행 중인 안내에 반영된다(_save). 여기서는 권한만 확인한다.
  Future<void> _setOverlayGuide(bool value) async {
    if (!value) {
      setState(() => _overlayGuide = false);
      return;
    }
    setState(() => _busy = true);
    final allowed = await GuideOverlayPlatform.requestPermission();
    if (!mounted) return;
    setState(() {
      _busy = false;
      _overlayGuide = allowed;
      _status = allowed ? '다른 앱 위 미니 지도 권한이 허용됐습니다.' : '다른 앱 위 표시 권한이 필요합니다.';
    });
  }

  Future<void> _check() async {
    setState(() {
      _busy = true;
      _status = '확인 중…';
    });
    final s = _current;
    final api = ApiClient(baseUrl: s.baseUrl, token: s.token);
    // 응답 전에 화면을 나가면 State 가 폐기되므로 await 뒤 setState 마다 mounted 를 본다.
    if (s.baseUrl.isEmpty) {
      setState(() {
        _busy = false;
        _status = '서버 주소를 먼저 입력하세요';
      });
      return;
    }
    // 실패 원인을 구분해 보여 준다: 주소에 닿지 않음 / 토큰 없음·만료 / 그 외. (허용목록 밖 계정은 로그인 때 403 —
    // 토큰이 발급되지 않아 여기서는 401 로 나타난다. 그 안내는 _loginGoogle 이 한다.)
    try {
      final h = await api.health();
      final me = await api.me();
      final sp = await api.mySpeed();
      if (!mounted) return;
      setState(
        () => _status =
            '서버 OK (db ${h['db']}, otp ${h['otp']}, 버전 ${h['version'] ?? '?'}) · 사용자 ${me['user_id']}\n'
            '내 속도 — 걷기 ${sp['walk']?.label} · 자전거 ${sp['bicycle']?.label}',
      );
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _status = switch (e.status) {
        401 => '서버는 닿았지만 토큰이 없거나 만료됨 — Google 로그인(개발은 devtoken)으로 다시 받으세요',
        _ => '실패: $e',
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _status = '서버에 닿지 않음: ${s.baseUrl} — 주소·PC 서버·Tailscale 상태를 확인하세요 ($e)');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  /// Google 계정으로 로그인해 받은 서버 JWT 를 토큰 칸에 넣고 저장한 뒤, _save 와 같이 새 설정을 돌려주며 화면을 닫는다
  /// (돌려주지 않으면 홈 화면은 앱을 다시 켤 때까지 옛 토큰을 쓴다).
  Future<void> _loginGoogle() async {
    setState(() {
      _busy = true;
      _status = 'Google 로그인 중…';
    });
    try {
      final res = await widget.login(
        ApiClient(baseUrl: normalizeBaseUrl(_url.text), token: ''),
      );
      if (!mounted) return;
      if (res == null) {
        setState(() => _status = '로그인 취소');
        return;
      }
      _token.text = res.token;
      await widget.settingsStore.save(_current);
      if (!mounted) return;
      Navigator.pop(context, _current);
    } on ApiException catch (e) {
      if (!mounted) return;
      // 403 = 서버 허용목록(AUTH_ALLOWED_EMAILS) 밖 계정. 토큰이 발급되지 않으므로 여기서만 알려 줄 수 있다.
      setState(() => _status = e.status == 403
          ? '로그인 실패: 이 계정은 이 서버의 허용목록에 없습니다(서버 AUTH_ALLOWED_EMAILS 확인)'
          : '로그인 실패: $e');
    } catch (e) {
      if (!mounted) return;
      setState(() => _status = '로그인 실패: $e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _save() async {
    await widget.settingsStore.save(_current);
    // 안내가 진행 중이면 미니 지도 켜짐·진하기를 그 안내에 바로 반영한다(다음 안내까지 기다리지 않게).
    await ActiveGuide.instance.current?.refreshOverlaySetting();
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
            readOnly: Env.isProd,
            decoration: InputDecoration(
              labelText: '서버 주소',
              helperText: Env.isProd
                  ? '운영 빌드에 고정된 주소라 바꿀 수 없습니다'
                  : '개발 스택 — 폰: http://<PC>.<tailnet>.ts.net:8082 · 에뮬레이터: http://10.0.2.2:8082 · '
                      'USB(adb reverse): http://127.0.0.1:8082',
              errorText: Env.isProd && _url.text.isEmpty ? '빌드에 서버 주소가 없습니다(env.prod.json 의 API_BASE_URL)' : null,
            ),
            keyboardType: TextInputType.url,
          ),
          const SizedBox(height: 12),
          FilledButton.tonalIcon(
            onPressed: _busy ? null : _loginGoogle,
            icon: const Icon(Icons.login),
            label: const Text('Google 계정으로 로그인'),
          ),
          const SizedBox(height: 12),
          // 개발용 토큰 붙여넣기는 dev 에만. prod 는 Google 로그인이 채운 토큰만 보이지 않게 들고 있다
          // (첫 실행 때 이전 앱이 남긴 토큰은 SettingsStore.load 가 버린다).
          if (!Env.isProd)
            TextField(
              controller: _token,
              decoration: const InputDecoration(
                labelText: '토큰(JWT)',
                helperText: 'Google 로그인이 채운다. 개발용: PC 에서 go run ./cmd/devtoken <이름> 으로 발급해 붙여 넣기',
              ),
              maxLines: 3,
            ),
          const SizedBox(height: 8),
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            title: const Text('음성 안내'),
            subtitle: const Text('안내 중 구간 전환·방향 전환·하차를 소리로 읽어 줍니다'),
            value: _voiceGuide,
            onChanged: _busy ? null : (v) => setState(() => _voiceGuide = v),
          ),
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            title: const Text('다른 앱 위 미니 지도'),
            subtitle: const Text(
              '안내 중 다른 앱으로 이동하면 반투명 지도와 다음 행동을 표시합니다. 터치는 아래 앱으로 전달됩니다.',
            ),
            value: _overlayGuide,
            onChanged: _busy ? null : _setOverlayGuide,
          ),
          // 미니 지도 진하기. 화면은 5~100%, 내부 알파는 Settings.overlayAlphaOf 로 0.04~0.8(100% = Android 터치 차단 상한).
          ListTile(
            contentPadding: EdgeInsets.zero,
            title: Text('미니 지도 진하기 ${Settings.overlayPercentOf(_overlayOpacity)}%'),
            subtitle: Slider(
              value: Settings.overlayPercentOf(_overlayOpacity).toDouble(),
              min: Settings.minOverlayPercent.toDouble(),
              max: 100,
              divisions: 19,
              label: '${Settings.overlayPercentOf(_overlayOpacity)}%',
              onChanged: _busy || !_overlayGuide
                  ? null
                  : (v) => setState(() => _overlayOpacity = Settings.overlayAlphaOf(v)),
              // 안내 중이면 떠 있는 창에 바로 반영된다(저장은 "저장" 버튼).
              onChangeEnd: (v) => GuideOverlayPlatform.setOpacity(Settings.overlayAlphaOf(v)),
            ),
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              OutlinedButton(
                onPressed: _busy ? null : _check,
                child: const Text('연결 확인'),
              ),
              const SizedBox(width: 12),
              FilledButton(
                onPressed: _busy ? null : _save,
                child: const Text('저장'),
              ),
            ],
          ),
          const SizedBox(height: 12),
          Text(_status),
        ],
      ),
    );
  }
}
