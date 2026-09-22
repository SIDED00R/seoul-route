import 'package:flutter/material.dart';

import '../api/client.dart';
import '../models/favorite_place.dart';
import '../models/place.dart';
import 'place_search_screen.dart';

class FavoritePlacesScreen extends StatefulWidget {
  const FavoritePlacesScreen({super.key, required this.api});

  final ApiClient api;

  @override
  State<FavoritePlacesScreen> createState() => _FavoritePlacesScreenState();
}

class _FavoritePlacesScreenState extends State<FavoritePlacesScreen> {
  List<FavoritePlace>? _favorites;
  bool _busy = false;
  String _error = '';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    if (!mounted) return; // 저장·삭제 응답을 기다리는 동안 화면을 닫았으면 갱신할 State 가 없다
    setState(() {
      _busy = true;
      _error = '';
    });
    try {
      final values = await widget.api.favoritePlaces();
      if (mounted) setState(() => _favorites = values);
    } catch (e) {
      if (mounted) setState(() => _error = '불러오기 실패: $e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _add() async {
    final place = await Navigator.push<Place>(
      context,
      MaterialPageRoute(
        builder: (_) => PlaceSearchScreen(api: widget.api, title: '즐겨찾기 장소'),
      ),
    );
    if (place == null || !mounted) return;
    final draft = await _editDialog(place: place);
    if (draft == null) return;
    await _save(draft);
  }

  Future<FavoritePlace?> _editDialog({
    FavoritePlace? current,
    required Place place,
  }) => showDialog<FavoritePlace>(
    context: context,
    builder: (_) => _FavoriteEditDialog(current: current, place: place),
  );

  Future<void> _save(FavoritePlace favorite) async {
    setState(() {
      _busy = true;
      _error = '';
    });
    try {
      if (favorite.id.isEmpty) {
        await widget.api.createFavoritePlace(favorite);
      } else {
        await widget.api.updateFavoritePlace(favorite);
      }
      await _load();
    } catch (e) {
      if (mounted) setState(() => _error = '저장 실패: $e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _edit(FavoritePlace favorite) async {
    final changed = await _editDialog(current: favorite, place: favorite.place);
    if (changed != null) await _save(changed);
  }

  Future<void> _delete(FavoritePlace favorite) async {
    final yes = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('${favorite.label} 삭제'),
        content: const Text('이 즐겨찾기를 삭제할까요?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('취소'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('삭제'),
          ),
        ],
      ),
    );
    if (yes != true || !mounted) return;
    setState(() {
      _busy = true;
      _error = '';
    });
    try {
      await widget.api.deleteFavoritePlace(favorite.id);
      await _load();
    } catch (e) {
      if (mounted) setState(() => _error = '삭제 실패: $e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('자주 가는 곳'),
      actions: [
        IconButton(
          onPressed: _busy ? null : _add,
          tooltip: '장소 추가',
          icon: const Icon(Icons.add),
        ),
      ],
    ),
    body: RefreshIndicator(
      onRefresh: _load,
      child: ListView(
        children: [
          if (_busy && _favorites == null)
            const Center(
              child: Padding(
                padding: EdgeInsets.all(24),
                child: CircularProgressIndicator(),
              ),
            ),
          if (_error.isNotEmpty)
            Padding(padding: const EdgeInsets.all(16), child: Text(_error)),
          if (_favorites != null && _favorites!.isEmpty && !_busy)
            const Padding(
              padding: EdgeInsets.all(24),
              child: Text(
                '오른쪽 위 + 버튼으로 집·회사·자주 가는 장소를 추가하세요.',
                textAlign: TextAlign.center,
              ),
            ),
          for (final favorite in _favorites ?? const <FavoritePlace>[])
            ListTile(
              leading: Icon(_icon(favorite.kind)),
              title: Text(favorite.label),
              subtitle: Text(
                '${favorite.place.name}\n${favorite.place.address}',
              ),
              isThreeLine: favorite.place.address.isNotEmpty,
              onTap: _busy ? null : () => _edit(favorite),
              trailing: IconButton(
                tooltip: '삭제',
                icon: const Icon(Icons.delete_outline),
                onPressed: _busy ? null : () => _delete(favorite),
              ),
            ),
        ],
      ),
    ),
  );

  static IconData _icon(FavoriteKind kind) => switch (kind) {
    FavoriteKind.home => Icons.home,
    FavoriteKind.work => Icons.business,
    FavoriteKind.custom => Icons.star,
  };
}

class _FavoriteEditDialog extends StatefulWidget {
  const _FavoriteEditDialog({required this.place, this.current});

  final Place place;
  final FavoritePlace? current;

  @override
  State<_FavoriteEditDialog> createState() => _FavoriteEditDialogState();
}

class _FavoriteEditDialogState extends State<_FavoriteEditDialog> {
  late FavoriteKind _kind;
  late final TextEditingController _label;

  @override
  void initState() {
    super.initState();
    _kind = widget.current?.kind ?? FavoriteKind.custom;
    _label = TextEditingController(
      text: widget.current?.label ?? widget.place.name,
    );
  }

  @override
  void dispose() {
    _label.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    title: Text(widget.current == null ? '즐겨찾기 추가' : '즐겨찾기 수정'),
    content: Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(widget.place.name, style: Theme.of(context).textTheme.titleMedium),
        if (widget.place.address.isNotEmpty) Text(widget.place.address),
        const SizedBox(height: 16),
        DropdownButtonFormField<FavoriteKind>(
          initialValue: _kind,
          decoration: const InputDecoration(labelText: '종류'),
          items: [
            for (final kind in FavoriteKind.values)
              DropdownMenuItem(value: kind, child: Text(kind.label)),
          ],
          onChanged: (value) {
            if (value != null) setState(() => _kind = value);
          },
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _label,
          maxLength: 20,
          autofocus: widget.current != null,
          decoration: const InputDecoration(labelText: '표시 이름'),
        ),
      ],
    ),
    actions: [
      TextButton(
        onPressed: () => Navigator.pop(context),
        child: const Text('취소'),
      ),
      FilledButton(
        onPressed: () {
          final value = _label.text.trim();
          if (value.isEmpty) return;
          Navigator.pop(
            context,
            FavoritePlace(
              id: widget.current?.id ?? '',
              kind: _kind,
              label: value,
              place: widget.place,
            ),
          );
        },
        child: const Text('저장'),
      ),
    ],
  );
}
