/// 직선거리 표시. 1km 미만은 m, 이상은 소수 한 자리 km("850m", "1.2km").
String distanceLabel(int meters) => meters < 1000 ? '${meters}m' : '${(meters / 1000).toStringAsFixed(1)}km';
