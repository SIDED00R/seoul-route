import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';

import '../api/client.dart';
import '../guide/active_guide.dart';
import '../guide/replace_guide_confirm.dart';
import '../models/itinerary.dart';
import '../models/plan_request.dart';
import '../util/leg_names.dart';
import '../util/polyline.dart';
import '../widgets/mode_icon.dart';
import 'guide_screen.dart';

/// 경로 상세: VWorld 타일(서버 프록시) 위에 leg 폴리라인·출발/경유/도착 마커, 아래에 leg 목록.
class DetailScreen extends StatelessWidget {
  const DetailScreen({
    super.key,
    required this.api,
    required this.request,
    required this.itinerary,
    required this.index,
  });

  final ApiClient api;
  final PlanRequest request;
  final Itinerary itinerary;
  final int index;

  /// 안내를 시작한다. 다른 여정으로 이미 안내 중이면 물어보고, 그 안내를 끝낸 뒤에 시작한다 —
  /// 안내는 한 번에 하나뿐이다(위치 스트림·알림창이 하나).
  Future<void> _startGuide(BuildContext context) async {
    final running = ActiveGuide.instance.current;
    if (running != null && !running.ended && !identical(running.itinerary, itinerary)) {
      if (!await confirmReplaceGuide(context)) return;
      await running.end();
      ActiveGuide.instance.clear();
    }
    if (!context.mounted) return;
    await Navigator.push(
      context,
      MaterialPageRoute(builder: (_) => GuideScreen(api: api, request: request, itinerary: itinerary)),
    );
  }

  @override
  Widget build(BuildContext context) {
    final polylines = <Polyline>[];
    final all = <LatLng>[];
    for (final leg in itinerary.legs) {
      final pts = leg.polyline.isEmpty
          ? [LatLng(leg.fromLat, leg.fromLon), LatLng(leg.toLat, leg.toLon)]
          : decodePolyline(leg.polyline);
      all.addAll(pts);
      polylines.add(Polyline(
        points: pts,
        color: modeColor(leg),
        strokeWidth: leg.mode == 'WALK' ? 4 : 6,
        pattern: leg.mode == 'WALK' ? const StrokePattern.dotted() : const StrokePattern.solid(),
      ));
    }
    final markers = <Marker>[
      _marker(LatLng(request.origin.lat, request.origin.lon), Icons.trip_origin, Colors.green),
      for (final v in request.via) _marker(LatLng(v.lat, v.lon), Icons.flag, Colors.orange),
      _marker(LatLng(request.destination.lat, request.destination.lon), Icons.place, Colors.red),
      for (final leg in itinerary.legs)
        if (leg.rentedBike) ...[
          _marker(LatLng(leg.fromLat, leg.fromLon), Icons.pedal_bike, Colors.green.shade900),
          _marker(LatLng(leg.toLat, leg.toLon), Icons.local_parking, Colors.green.shade900),
        ],
    ];
    if (all.isEmpty) {
      all.addAll([
        LatLng(request.origin.lat, request.origin.lon),
        LatLng(request.destination.lat, request.destination.lon),
      ]);
    }
    return Scaffold(
      appBar: AppBar(
        title: Text('후보 $index · ${itinerary.minutes}분'
            '${itinerary.realtimeLabel == null ? '' : ' · ${itinerary.realtimeLabel}'}'),
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () => _startGuide(context),
        icon: const Icon(Icons.navigation),
        label: const Text('안내 시작'),
      ),
      body: Column(
        children: [
          Expanded(
            flex: 3,
            child: FlutterMap(
              options: MapOptions(
                initialCameraFit: CameraFit.bounds(
                  bounds: LatLngBounds.fromPoints(all),
                  padding: const EdgeInsets.all(32),
                ),
              ),
              children: [
                TileLayer(
                  urlTemplate: api.tileUrlTemplate,
                  tileProvider: NetworkTileProvider(headers: api.tileHeaders),
                  userAgentPackageName: 'kr.seoulroute.seoul_route',
                  // VWorld 는 256px 래스터라 560dpi 화면에서 흐리다. 템플릿에 {r} 이 없으면 flutter_map 이
                  // 한 단계 높은 줌 타일을 받아 절반 크기로 그리는 시뮬레이션 모드로 동작한다(글자는 작아진다).
                  retinaMode: true,
                ),
                PolylineLayer(polylines: polylines),
                MarkerLayer(markers: markers),
                const RichAttributionWidget(
                  attributions: [TextSourceAttribution('© VWorld(국토교통부) · OSM · 서울시 따릉이')],
                ),
              ],
            ),
          ),
          Expanded(
            flex: 2,
            child: ListView.builder(
              itemCount: itinerary.legs.length,
              itemBuilder: (context, i) {
                final leg = itinerary.legs[i];
                return ListTile(
                  dense: true,
                  leading: Icon(modeIcon(leg), color: modeColor(leg)),
                  title: Text('${leg.label} · ${(leg.durationSec / 60).round()}분 · '
                      '${(leg.distanceM / 1000).toStringAsFixed(1)}km'),
                  subtitle: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('${_name(leg.fromName, leg.fromLat, leg.fromLon)} → '
                          '${_name(leg.toName, leg.toLat, leg.toLon)}'),
                      // 앞뒤 차·배차: 실시간 다음 차 / 시간표 앞·뒤 열차 / 배차간격
                      if (leg.scheduleLabel != null)
                        Text(leg.scheduleLabel!, style: TextStyle(color: Colors.teal.shade700)),
                      // 도보·따릉이: 지나는 신호 횡단보도와 그 대기(소요에 포함)
                      if (leg.crossingLabel != null)
                        Text(leg.crossingLabel!, style: TextStyle(color: Colors.orange.shade800)),
                    ],
                  ),
                  isThreeLine: leg.scheduleLabel != null || leg.crossingLabel != null,
                );
              },
            ),
          ),
        ],
      ),
    );
  }

  String _name(String n, double lat, double lon) => legEndpointName(request, n, lat, lon);

  Marker _marker(LatLng p, IconData icon, Color color) =>
      Marker(point: p, width: 32, height: 32, child: Icon(icon, color: color, size: 28));
}
