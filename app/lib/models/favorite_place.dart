import 'place.dart';

enum FavoriteKind {
  home('home', '집'),
  work('work', '회사'),
  custom('custom', '기타');

  const FavoriteKind(this.value, this.label);
  final String value;
  final String label;

  static FavoriteKind fromValue(String value) => FavoriteKind.values.firstWhere(
    (v) => v.value == value,
    orElse: () => FavoriteKind.custom,
  );
}

class FavoritePlace {
  const FavoritePlace({
    required this.id,
    required this.kind,
    required this.label,
    required this.place,
  });

  factory FavoritePlace.fromJson(Map<String, dynamic> json) => FavoritePlace(
    id: json['id'] as String,
    kind: FavoriteKind.fromValue(json['kind'] as String? ?? 'custom'),
    label: json['label'] as String,
    place: Place.fromJson(json['place'] as Map<String, dynamic>),
  );

  final String id;
  final FavoriteKind kind;
  final String label;
  final Place place;

  Map<String, dynamic> toInputJson() => {
    'kind': kind.value,
    'label': label,
    'place': {
      'name': place.name,
      'address': place.address,
      'category': place.category,
      'lat': place.lat,
      'lon': place.lon,
    },
  };
}

class GuideLandmark {
  const GuideLandmark({
    required this.name,
    required this.lat,
    required this.lon,
    required this.distanceM,
  });

  factory GuideLandmark.fromJson(Map<String, dynamic> json) => GuideLandmark(
    name: json['name'] as String,
    lat: (json['lat'] as num).toDouble(),
    lon: (json['lon'] as num).toDouble(),
    distanceM: (json['distance_m'] as num).toInt(),
  );

  final String name;
  final double lat;
  final double lon;
  final int distanceM;
}
