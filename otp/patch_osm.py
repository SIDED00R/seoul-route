"""osm-overrides.json 의 오버라이드를 data/seoul.osm.pbf 에 적용한다.

extract_seoul.py 뒤, 그래프 빌드 전에 실행한다. OSM 원본의 낡은 태그(예: 끝난 공사의 foot=no)나 빠진 연결을
원본 수정과 재배포를 기다리지 않고 바로잡는 용도다. 같은 목록을 다시 적용해도 결과는 같다.

목록 항목 두 종류(공통: "reason" 근거, "checked" 확인일):
  {"way": id, "tags": {키: 값}}                 기존 way 의 태그를 덮어쓴다(나머지 태그는 유지)
  {"add_way": id, "nodes": [노드 id…], "tags": {…}}  파일에 있는 노드들을 잇는 way 를 새로 만든다.
                                                  id 는 OSM 에 없는 값(9e11 대)을 쓴다
목록의 way·노드가 파일에 없으면 실패한다 — OSM 을 다시 받으면 way 가 갈라지거나 지워질 수 있으니 목록을
다시 확인한다. 이미 패치된 파일에 다시 돌리면 add_way 는 목록의 것으로 바뀐다.
"""

import json
import os
import sys
import time
from dataclasses import dataclass, field

import osmium

DIR = os.path.dirname(os.path.abspath(__file__))
PBF = os.path.join(DIR, "data", "seoul.osm.pbf")
OVERRIDES = os.path.join(DIR, "osm-overrides.json")


@dataclass
class Overrides:
    tags: dict[int, dict[str, str]] = field(default_factory=dict)  # way id → 덮어쓸 태그
    add: dict[int, tuple[list[int], dict[str, str]]] = field(default_factory=dict)  # 새 way id → (노드, 태그)


def _check_tags(label: str, tags) -> dict[str, str]:
    if not isinstance(tags, dict) or not tags or any(not isinstance(v, str) for v in tags.values()):
        raise ValueError(f"{label}: tags 는 비지 않은 문자열 사전이어야 한다")
    return tags


def load_overrides(path: str) -> Overrides:
    with open(path, encoding="utf-8") as f:
        items = json.load(f)
    out = Overrides()
    for it in items:
        if "way" in it:
            wid = int(it["way"])
            if wid in out.tags:
                raise ValueError(f"way {wid} 가 목록에 두 번 있다")
            out.tags[wid] = _check_tags(f"way {wid}", it["tags"])
        elif "add_way" in it:
            wid = int(it["add_way"])
            nodes = [int(n) for n in it["nodes"]]
            if wid in out.add or len(nodes) < 2:
                raise ValueError(f"add_way {wid}: id 중복이거나 노드가 2개 미만")
            out.add[wid] = (nodes, _check_tags(f"add_way {wid}", it["tags"]))
        else:
            raise ValueError(f"항목에 way 나 add_way 가 없다: {it}")
    return out


def apply(src: str, dst: str, ov: Overrides) -> tuple[set[int], set[int]]:
    """src 의 모든 객체를 dst 로 쓰되 ov.tags 의 way 는 태그를 덮어쓰고 ov.add 의 way 를 마지막 way 뒤에 만든다.

    (실제로 만난 태그 오버라이드 way id, 파일에 없던 add_way 노드 id) 를 돌려준다. add_way 의 id 가 이미
    파일에 있으면(이미 패치된 파일에 다시 적용) 그 way 를 목록의 것으로 바꾼다.
    """
    applied: set[int] = set()
    need = {n for nodes, _ in ov.add.values() for n in nodes}
    added = False
    with osmium.SimpleWriter(dst) as writer:

        def flush_added() -> None:
            nonlocal added
            if added:
                return
            added = True
            for wid, (nodes, tags) in ov.add.items():
                writer.add(osmium.osm.mutable.Way(id=wid, nodes=nodes, tags=tags))

        for o in osmium.FileProcessor(src):
            if o.is_node():
                need.discard(o.id)
            elif o.is_way():
                if o.id in ov.add:
                    continue  # 이전 패치가 만든 way — flush_added 가 목록의 것으로 다시 쓴다
                if o.id in ov.tags:
                    tags = dict(o.tags)
                    tags.update(ov.tags[o.id])
                    writer.add(o.replace(tags=tags))
                    applied.add(o.id)
                    continue
            else:
                flush_added()
            writer.add(o)
        flush_added()
    return applied, need


def main() -> int:
    if not os.path.exists(PBF):
        print(f"입력 없음: {PBF} — extract_seoul.py 를 먼저 실행한다", file=sys.stderr)
        return 1
    ov = load_overrides(OVERRIDES)
    tmp = os.path.join(os.path.dirname(PBF), "seoul.patching.osm.pbf")  # osmium 은 확장자로 형식을 정한다
    if os.path.exists(tmp):
        os.remove(tmp)
    t0 = time.time()
    try:
        applied, missing_nodes = apply(PBF, tmp, ov)
    except ValueError as e:
        os.remove(tmp)
        print(f"{e} — {OVERRIDES} 를 다시 확인한다", file=sys.stderr)
        return 1
    missing = sorted(set(ov.tags) - applied)
    if missing or missing_nodes:
        os.remove(tmp)
        print(f"way 없음: {missing}, add_way 노드 없음: {sorted(missing_nodes)} — OSM 을 다시 받았으면 {OVERRIDES} 를 다시 확인한다",
              file=sys.stderr)
        return 1
    os.replace(tmp, PBF)
    for wid in sorted(applied):
        print(f"way {wid}: {ov.tags[wid]}")
    for wid, (nodes, tags) in ov.add.items():
        print(f"add_way {wid}: nodes {nodes} {tags}")
    print(f"완료 {PBF}: way {len(applied)}개 덮어씀, {len(ov.add)}개 추가, {time.time() - t0:.0f}초")
    return 0


if __name__ == "__main__":
    sys.exit(main())
