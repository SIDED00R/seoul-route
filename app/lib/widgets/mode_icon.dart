import 'package:flutter/material.dart';

import '../models/itinerary.dart';

/// leg 수단별 아이콘과 색. 목록과 지도 폴리라인이 같은 색을 쓴다.
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

Color modeColor(Leg leg) {
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
