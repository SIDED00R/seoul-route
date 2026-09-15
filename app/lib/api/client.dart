import 'dart:convert';

import 'package:http/http.dart' as http;

import '../auth/google_login.dart';
import '../models/itinerary.dart';
import '../models/place.dart';
import '../models/plan_request.dart';
import '../models/trace_sample.dart';

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

  /// 서버가 앱에 알려주는 Google 웹 클라이언트 ID(무인증). 서버에 미설정이면 빈 문자열.
  Future<String> googleClientId() async {
    final r = await http
        .get(Uri.parse('$baseUrl/auth/config'))
        .timeout(const Duration(seconds: 10));
    return (_decode(r)['google_client_id'] as String?) ?? '';
  }

  /// Google ID 토큰 → 서버 JWT(무인증).
  Future<LoginResult> loginGoogle(String idToken) async {
    final r = await http
        .post(Uri.parse('$baseUrl/auth/google'),
            headers: {'Content-Type': 'application/json'}, body: jsonEncode({'id_token': idToken}))
        .timeout(const Duration(seconds: 15));
    final j = _decode(r);
    return LoginResult(token: j['token'] as String, userId: j['user_id'] as String);
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

  /// 안내 1회 = trip. 서버가 발급한 id 로만 샘플을 올릴 수 있다.
  Future<String> startTrip() async {
    final r = await http
        .post(Uri.parse('$baseUrl/trips'), headers: _headers)
        .timeout(const Duration(seconds: 10));
    return _decode(r)['trip_id'] as String;
  }

  Future<void> uploadTraces(String tripId, List<TraceSample> samples) async {
    final r = await http
        .post(Uri.parse('$baseUrl/trips/$tripId/traces'),
            headers: _headers, body: jsonEncode({'samples': samples.map((s) => s.toJson()).toList()}))
        .timeout(const Duration(seconds: 15));
    _decode(r);
  }

  /// trip 종료. 응답의 trip(이번 안내의 수단별 속도·채택 여부)과 profile(갱신된 내 속도)을 그대로 돌려준다.
  Future<Map<String, dynamic>> endTrip(String tripId) async {
    final r = await http
        .post(Uri.parse('$baseUrl/trips/$tripId/end'), headers: _headers)
        .timeout(const Duration(seconds: 15));
    return _decode(r);
  }

  Future<Map<String, SpeedProfile>> mySpeed() async {
    final r = await http
        .get(Uri.parse('$baseUrl/users/me/speed'), headers: _headers)
        .timeout(const Duration(seconds: 10));
    final j = _decode(r);
    return {
      for (final mode in ['walk', 'bicycle'])
        if (j[mode] != null) mode: SpeedProfile.fromJson(j[mode] as Map<String, dynamic>),
    };
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
