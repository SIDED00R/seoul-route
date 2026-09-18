import 'package:flutter/material.dart';

import '../models/itinerary.dart';

/// leg 아이콘과 색(노선 색이 있으면 그 색, 없으면 수단별 기본색). 목록과 지도 폴리라인이 같은 색을 쓴다.
IconData modeIcon(Leg leg) {
  switch (leg.mode) {
    case 'WALK':
      return Icons.directions_walk;
    case 'BICYCLE':
      return Icons.pedal_bike;
    case 'BUS':
      return Icons.directions_bus;
    case 'SUBWAY':
    case 'RAIL':
      return Icons.subway;
    default:
      return Icons.commute;
  }
}

/// leg 색. 서버가 노선 색(GTFS route_color)을 줬으면 그 색, 아니면 수단별 기본 팔레트.
Color modeColor(Leg leg) => parseHexColor(leg.color) ?? defaultModeColor(leg);

/// 노선 색 위에 얹을 글자색. 서버가 준 값이 없으면 흰색.
Color legTextColor(Leg leg) => parseHexColor(leg.textColor) ?? Colors.white;

/// "00A84D" 같은 6자리 16진수를 색으로. 형식이 아니면 null.
Color? parseHexColor(String hex) {
  if (hex.length != 6) return null;
  final v = int.tryParse(hex, radix: 16);
  return v == null ? null : Color(0xFF000000 | v);
}

Color defaultModeColor(Leg leg) {
  switch (leg.mode) {
    case 'WALK':
      return Colors.grey.shade700;
    case 'BICYCLE':
      return Colors.green.shade700;
    case 'BUS':
      return Colors.blue.shade700;
    case 'SUBWAY':
    case 'RAIL':
      return Colors.deepOrange.shade700;
    default:
      return Colors.purple.shade700;
  }
}
