import 'dart:async';

import 'package:flutter/material.dart';

import '../api/client.dart';
import '../models/place.dart';

/// 장소 검색. 입력 후 400ms 뒤 서버(/places/search → 카카오)를 호출하고, 고르면 Place 를 돌려준다.
class PlaceSearchScreen extends StatefulWidget {
  const PlaceSearchScreen({super.key, required this.api, required this.title});

  final ApiClient api;
  final String title;

  @override
  State<PlaceSearchScreen> createState() => _PlaceSearchScreenState();
}

class _PlaceSearchScreenState extends State<PlaceSearchScreen> {
  final _ctl = TextEditingController();
  Timer? _debounce;
  List<Place> _results = const [];
  String _error = '';
  bool _busy = false;
  int _gen = 0; // 검색 세대. 늦게 도착한 이전 요청의 응답은 버린다.

  void _onChanged(String q) {
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 400), () => _search(q.trim()));
  }

  Future<void> _search(String q) async {
    final g = ++_gen;
    if (q.isEmpty) {
      setState(() {
        _results = const [];
        _busy = false; // 진행 중이던 요청은 세대가 바뀌어 스피너를 끄지 못하므로 여기서 끈다
      });
      return;
    }
    setState(() {
      _busy = true;
      _error = '';
    });
    try {
      final r = await widget.api.searchPlaces(q);
      if (!mounted || g != _gen) return;
      setState(() => _results = r);
    } catch (e) {
      if (!mounted || g != _gen) return;
      setState(() => _error = '검색 실패: $e');
    } finally {
      if (mounted && g == _gen) setState(() => _busy = false);
    }
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _ctl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(widget.title)),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.all(12),
            child: TextField(
              controller: _ctl,
              autofocus: true,
              onChanged: _onChanged,
              onSubmitted: (q) => _search(q.trim()),
              decoration: InputDecoration(
                hintText: '장소·주소 (서울)',
                prefixIcon: const Icon(Icons.search),
                suffixIcon: _busy
                    ? const Padding(
                        padding: EdgeInsets.all(12),
                        child: SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)),
                      )
                    : null,
              ),
            ),
          ),
          if (_error.isNotEmpty) Padding(padding: const EdgeInsets.all(8), child: Text(_error)),
          Expanded(
            child: ListView.builder(
              itemCount: _results.length,
              itemBuilder: (context, i) {
                final p = _results[i];
                return ListTile(
                  title: Text(p.name),
                  subtitle: Text(p.category.isEmpty ? p.address : '${p.category} · ${p.address}'),
                  onTap: () => Navigator.pop(context, p),
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}
