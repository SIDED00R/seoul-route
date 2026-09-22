import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';

import '../settings/settings_store.dart';

class GuideOverlaySnapshot {
  const GuideOverlaySnapshot({
    required this.baseUrl,
    required this.token,
    required this.points,
    required this.here,
    required this.turn,
    required this.now,
    required this.next,
    required this.remainMin,
    required this.eta,
    required this.color,
  });

  final String baseUrl;
  final String token;
  final List<LatLng> points;
  final LatLng? here;
  final LatLng? turn;
  final String now;
  final String next;
  final int remainMin;
  final String eta;
  final int color;

  Map<String, dynamic> toMap() => {
    'base_url': baseUrl,
    'token': token,
    'points': [
      for (final p in points) [p.latitude, p.longitude],
    ],
    if (here != null) 'here': [here!.latitude, here!.longitude],
    if (turn != null) 'turn': [turn!.latitude, turn!.longitude],
    'now': now,
    'next': next,
    'remain_min': remainMin,
    'eta': eta,
    'color': color,
  };

  factory GuideOverlaySnapshot.fromMap(Map<dynamic, dynamic> map) {
    LatLng? point(dynamic value) {
      if (value is! List || value.length < 2) return null;
      return LatLng((value[0] as num).toDouble(), (value[1] as num).toDouble());
    }

    return GuideOverlaySnapshot(
      baseUrl: map['base_url'] as String? ?? '',
      token: map['token'] as String? ?? '',
      points: (map['points'] as List<dynamic>? ?? const [])
          .map(point)
          .whereType<LatLng>()
          .toList(),
      here: point(map['here']),
      turn: point(map['turn']),
      now: map['now'] as String? ?? '',
      next: map['next'] as String? ?? '',
      remainMin: (map['remain_min'] as num?)?.toInt() ?? 0,
      eta: map['eta'] as String? ?? '',
      color: (map['color'] as num?)?.toInt() ?? 0xFF00695C,
    );
  }
}

class GuideOverlayPlatform {
  static const _channel = MethodChannel('seoul_route/guide_overlay');

  static Future<bool> requestPermission() async {
    try {
      return await _channel.invokeMethod<bool>('requestPermission') ?? false;
    } on MissingPluginException {
      return false;
    } on PlatformException {
      return false;
    }
  }

  /// [opacity] 는 창 불투명도(Settings.overlayOpacity). 네이티브가 기기 터치 상한으로 한 번 더 자른다.
  /// 켤 때 네이티브→앱 호출(미니 지도 손잡이 슬라이더의 값)을 받을 핸들러도 건다.
  static Future<void> setEnabled(bool enabled, {double? opacity}) async {
    if (enabled) _channel.setMethodCallHandler(_onPlatformCall);
    try {
      await _channel.invokeMethod<void>('setEnabled', {
        'enabled': enabled,
        'opacity': ?opacity,
      });
    } on MissingPluginException {
      // Android 외 플랫폼·테스트에서는 오버레이 없이 안내한다.
    } on PlatformException {
      // 오버레이 실패가 안내를 중단시키지 않는다.
    }
  }

  /// 네이티브 손잡이 슬라이더에서 손을 뗀 값(opacityChanged)을 설정에 저장한다. 미니 지도에는 이미 반영돼 있다.
  static Future<void> _onPlatformCall(MethodCall call) async {
    if (call.method != 'opacityChanged' || call.arguments is! num) return;
    await SettingsStore.saveOverlayOpacity((call.arguments as num).toDouble());
  }

  /// 떠 있는 창에도 바로 적용된다(설정 슬라이더가 부른다).
  static Future<void> setOpacity(double opacity) async {
    try {
      await _channel.invokeMethod<void>('setOpacity', {'opacity': opacity});
    } on MissingPluginException {
      // 오버레이 없음
    } on PlatformException {
      // 오버레이 없음
    }
  }

  static Future<void> update(GuideOverlaySnapshot snapshot) async {
    try {
      await _channel.invokeMethod<void>('update', snapshot.toMap());
    } on MissingPluginException {
      // 오버레이 없이 안내
    } on PlatformException {
      // 오버레이 없이 안내
    }
  }

  static Future<void> stop() => setEnabled(false);
}

void runGuideOverlay() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const GuideOverlayApp());
}

class GuideOverlayApp extends StatelessWidget {
  const GuideOverlayApp({super.key});

  @override
  Widget build(BuildContext context) => MaterialApp(
    debugShowCheckedModeBanner: false,
    builder: (context, child) {
      final media = MediaQuery.of(context);
      return MediaQuery(
        data: media.copyWith(textScaler: const TextScaler.linear(1)),
        child: child!,
      );
    },
    home: const Material(
      type: MaterialType.transparency,
      child: GuideOverlayView(),
    ),
  );
}

class GuideOverlayView extends StatefulWidget {
  const GuideOverlayView({super.key});

  @override
  State<GuideOverlayView> createState() => _GuideOverlayViewState();
}

class _GuideOverlayViewState extends State<GuideOverlayView> {
  static const _channel = MethodChannel('seoul_route/guide_overlay_data');
  final MapController _map = MapController();
  GuideOverlaySnapshot? _snapshot;
  bool _mapReady = false;

  @override
  void initState() {
    super.initState();
    _channel.setMethodCallHandler((call) async {
      if (call.method != 'update' || call.arguments is! Map) return;
      final snapshot = GuideOverlaySnapshot.fromMap(
        call.arguments as Map<dynamic, dynamic>,
      );
      if (!mounted) return;
      setState(() => _snapshot = snapshot);
      final center =
          snapshot.here ??
          snapshot.turn ??
          (snapshot.points.isEmpty ? null : snapshot.points.first);
      if (_mapReady && center != null) _map.move(center, 17);
    });
    unawaited(_channel.invokeMethod<void>('ready'));
  }

  @override
  void dispose() {
    _channel.setMethodCallHandler(null);
    _map.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final snapshot = _snapshot;
    if (snapshot == null) {
      return const ColoredBox(
        color: Color(0xCCFFFFFF),
        child: Center(
          child: Text('안내 준비 중…', textScaler: TextScaler.noScaling),
        ),
      );
    }
    final center =
        snapshot.here ??
        snapshot.turn ??
        (snapshot.points.isEmpty
            ? const LatLng(37.5665, 126.978)
            : snapshot.points.first);
    return ColoredBox(
      color: const Color(0xE6FFFFFF),
      child: Column(
        children: [
          SizedBox(
            height: 170,
            child: FlutterMap(
              mapController: _map,
              options: MapOptions(
                initialCenter: center,
                initialZoom: 17,
                interactionOptions: const InteractionOptions(
                  flags: InteractiveFlag.none,
                ),
                onMapReady: () => _mapReady = true,
              ),
              children: [
                if (snapshot.baseUrl.isNotEmpty)
                  TileLayer(
                    urlTemplate: '${snapshot.baseUrl}/tiles/{z}/{x}/{y}.png',
                    tileProvider: NetworkTileProvider(
                      headers: {'Authorization': 'Bearer ${snapshot.token}'},
                    ),
                    userAgentPackageName: 'kr.seoulroute.seoul_route',
                    retinaMode: true,
                  ),
                if (snapshot.points.length > 1)
                  PolylineLayer(
                    polylines: [
                      Polyline(
                        points: snapshot.points,
                        color: Color(snapshot.color),
                        strokeWidth: 7,
                      ),
                    ],
                  ),
                MarkerLayer(
                  markers: [
                    if (snapshot.turn != null)
                      Marker(
                        point: snapshot.turn!,
                        width: 28,
                        height: 28,
                        child: const Icon(
                          Icons.turn_right,
                          color: Colors.orange,
                          size: 26,
                        ),
                      ),
                    if (snapshot.here != null)
                      Marker(
                        point: snapshot.here!,
                        width: 28,
                        height: 28,
                        child: const Icon(
                          Icons.my_location,
                          color: Colors.blue,
                          size: 25,
                        ),
                      ),
                  ],
                ),
                const RichAttributionWidget(
                  attributions: [TextSourceAttribution('© VWorld · OSM · 서울시')],
                ),
              ],
            ),
          ),
          Expanded(
            child: Padding(
              // 오른쪽은 네이티브 진하기 손잡이(44dp + 여백 8dp, GuideOverlayControls) 자리를 비워 둔다.
              padding: const EdgeInsets.fromLTRB(12, 6, 64, 6),
              child: Row(
                children: [
                  Icon(
                    Icons.navigation,
                    color: Color(snapshot.color),
                    size: 26,
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Column(
                      mainAxisAlignment: MainAxisAlignment.center,
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          snapshot.now,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                          textScaler: TextScaler.noScaling,
                          style: const TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.bold,
                          ),
                        ),
                        Text(
                          '다음: ${snapshot.next}',
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          textScaler: TextScaler.noScaling,
                          style: const TextStyle(fontSize: 12),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(width: 8),
                  Text(
                    '${snapshot.remainMin}분\n${snapshot.eta}',
                    textAlign: TextAlign.right,
                    textScaler: TextScaler.noScaling,
                    style: const TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}
