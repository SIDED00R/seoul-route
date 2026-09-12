import 'dart:convert';

import 'package:http/http.dart' as http;

import '../models/itinerary.dart';
import '../models/place.dart';
import '../models/plan_request.dart';

class ApiException implements Exception {
  const ApiException(this.status, this.message);
  final int status;
  final String message;

  @override
  String toString() => 'HTTP $status: $message';
}

// 백엔드 호출 한 곳. 키(카카오·VWorld)는 앱에 없고 전부 서버가 대신 호출한다.
class ApiClient {
  ApiClient({required this.baseUrl, required this.token});

  final String baseUrl; // 예: http://10.0.2.2:8081 (에뮬레이터) / http://192.168.x.x:8081 (실기기)
  final String token; // JWT

  static const planTimeout = Duration(seconds: 70); // 서버 상한 60초보다 길게

  Map<String, String> get _headers => {
        'Authorization': 'Bearer $token',
        'Content-Type': 'application/json',
      };

  /// 타일 URL 템플릿과 헤더. flutter_map 의 NetworkTileProvider 에 그대로 넘긴다.
  String get tileUrlTemplate => '$baseUrl/tiles/{z}/{x}/{y}.png';
  Map<String, String> get tileHeaders => {'Authorization': 'Bearer $token'};

  Future<Map<String, dynamic>> health() async {
    final r = await http
        .get(Uri.parse('$baseUrl/health'))
        .timeout(const Duration(seconds: 10));
    return _decode(r);
  }

  Future<Map<String, dynamic>> me() async {
    final r = await http
        .get(Uri.parse('$baseUrl/users/me'), headers: _headers)
        .timeout(const Duration(seconds: 10));
    return _decode(r);
  }

  Future<List<Place>> searchPlaces(String q) async {
    final uri = Uri.parse('$baseUrl/places/search').replace(queryParameters: {'q': q});
    final r = await http.get(uri, headers: _headers).timeout(const Duration(seconds: 15));
    final j = _decode(r);
    return ((j['places'] as List<dynamic>?) ?? const [])
        .map((e) => Place.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<PlanResult> plan(PlanRequest req) async {
    final r = await http
        .post(Uri.parse('$baseUrl/routes/plan'), headers: _headers, body: jsonEncode(req.toJson()))
        .timeout(planTimeout);
    return PlanResult.fromJson(_decode(r));
  }

  Map<String, dynamic> _decode(http.Response r) {
    final body = r.body.isEmpty ? <String, dynamic>{} : jsonDecode(utf8.decode(r.bodyBytes));
    if (r.statusCode >= 400) {
      final msg = body is Map && body['error'] != null ? body['error'] as String : r.reasonPhrase ?? '';
      throw ApiException(r.statusCode, msg);
    }
    return body as Map<String, dynamic>;
  }
}
