import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';

import '../api/client.dart';
import '../models/itinerary.dart';
import '../models/plan_request.dart';
import '../util/polyline.dart';
import '../widgets/mode_icon.dart';

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
                  subtitle: Text('${_name(leg.fromName, leg.fromLat, leg.fromLon)} → '
                      '${_name(leg.toName, leg.toLat, leg.toLon)}'),
                );
              },
            ),
          ),
        ],
      ),
    );
  }

  /// OTP 는 출발·도착을 "Origin"/"Destination" 으로 돌려준다. 구간 분할 경로에서는 경유지도 그렇게 오므로
  /// 좌표가 가장 가까운 지점(출발·경유·도착)의 이름으로 바꾼다.
  String _name(String n, double lat, double lon) {
    if (n != 'Origin' && n != 'Destination') return n;
    final candidates = [request.origin, ...request.via, request.destination];
    var best = candidates.first;
    var bestD = double.infinity;
    for (final p in candidates) {
      final d = (p.lat - lat) * (p.lat - lat) + (p.lon - lon) * (p.lon - lon);
      if (d < bestD) {
        bestD = d;
        best = p;
      }
    }
    return best.name;
  }

  Marker _marker(LatLng p, IconData icon, Color color) =>
      Marker(point: p, width: 32, height: 32, child: Icon(icon, color: color, size: 28));
}
