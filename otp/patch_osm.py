"""osm-overrides.json 의 오버라이드를 data/seoul.osm.pbf 에 적용한다.

extract_seoul.py 뒤, 그래프 빌드 전에 실행한다. OSM 원본의 낡은 태그(예: 끝난 공사의 foot=no)나 빠진 연결을
원본 수정과 재배포를 기다리지 않고 바로잡는 용도다. 같은 목록을 다시 적용해도 결과는 같다.

목록 항목 세 종류(공통: "reason" 근거, "checked" 확인일):
  {"way": id, "tags": {키: 값}}                 기존 way 의 태그를 덮어쓴다(나머지 태그는 유지)
  {"add_way": id, "nodes": [노드 id…], "tags": {…}}  파일에 있는 노드들을 잇는 way 를 새로 만든다.
                                                  id 는 OSM 에 없는 값(9e11 대)을 쓴다
  {"rule": {"match": {키: 값}, "name_regex": 정규식, "unset": [키…]}, "tags": {…}}
        match 태그가 모두 같고 name 이 정규식에 걸리며 unset 키가 없는 way 전부의 태그를 덮어쓴다. OSM 을 새로
        받아도 새 way 까지 덮이므로 같은 성격의 결함이 여럿일 때 쓴다. 규칙 → way 순으로 적용한다
목록의 way·노드가 파일에 없거나 규칙에 걸린 way 가 하나도 없으면 실패한다 — OSM 을 다시 받으면 way 가 갈라지거나
지워질 수 있으니 목록을 다시 확인한다. 이미 패치된 파일에 다시 돌리면 add_way 는 목록의 것으로 바뀌고, 규칙은 unset
키가 규칙이 쓰는 값과 같은 way 도 걸린 것으로 센다.
"""

import json
import os
import re
import sys
import time
from dataclasses import dataclass, field

import osmium

DIR = os.path.dirname(os.path.abspath(__file__))
PBF = os.path.join(DIR, "data", "seoul.osm.pbf")
OVERRIDES = os.path.join(DIR, "osm-overrides.json")


@dataclass
class Rule:
    match: dict[str, str]
    name_regex: re.Pattern | None
    unset: list[str]
    tags: dict[str, str]

    def matches(self, tags: dict[str, str]) -> bool:
        if any(tags.get(k) != v for k, v in self.match.items()):
            return False
        if self.name_regex and not self.name_regex.search(tags.get("name", "")):
            return False
        return all(k not in tags or tags[k] == self.tags.get(k) for k in self.unset)


@dataclass
class Overrides:
    tags: dict[int, dict[str, str]] = field(default_factory=dict)  # way id → 덮어쓸 태그
    add: dict[int, tuple[list[int], dict[str, str]]] = field(default_factory=dict)  # 새 way id → (노드, 태그)
    rules: list[Rule] = field(default_factory=list)


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
        elif "rule" in it:
            r = it["rule"]
            match = r.get("match") or {}
            if not match or any(not isinstance(v, str) for v in match.values()):
                raise ValueError(f"rule {r}: match 는 비지 않은 문자열 사전이어야 한다")
            regex = re.compile(r["name_regex"]) if r.get("name_regex") else None
            out.rules.append(Rule(match, regex, list(r.get("unset", [])), _check_tags(f"rule {r}", it["tags"])))
        else:
            raise ValueError(f"항목에 way·add_way·rule 이 없다: {it}")
    return out


def apply(src: str, dst: str, ov: Overrides) -> tuple[set[int], set[int], list[int]]:
    """src 의 모든 객체를 dst 로 쓰되 규칙·ov.tags 로 way 태그를 덮어쓰고 ov.add 의 way 를 마지막 way 뒤에 만든다.

    (실제로 만난 태그 오버라이드 way id, 파일에 없던 add_way 노드 id, 규칙마다 걸린 way 수) 를 돌려준다.
    add_way 의 id 가 이미 파일에 있으면(이미 패치된 파일에 다시 적용) 그 way 를 목록의 것으로 바꾼다.
    """
    applied: set[int] = set()
    rule_hits = [0] * len(ov.rules)
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
                tags = dict(o.tags)
                changed = False
                for i, r in enumerate(ov.rules):
                    if r.matches(tags):
                        tags.update(r.tags)
                        rule_hits[i] += 1
                        changed = True
                if o.id in ov.tags:
                    tags.update(ov.tags[o.id])
                    applied.add(o.id)
                    changed = True
                if changed:
                    writer.add(o.replace(tags=tags))
                    continue
            else:
                flush_added()
            writer.add(o)
        flush_added()
    return applied, need, rule_hits


def main() -> int:
    if not os.path.exists(PBF):
        print(f"입력 없음: {PBF} — extract_seoul.py 를 먼저 실행한다", file=sys.stderr)
        return 1
    ov = load_overrides(OVERRIDES)
    tmp = os.path.join(os.path.dirname(PBF), "seoul.patching.osm.pbf")  # osmium 은 확장자로 형식을 정한다
    if os.path.exists(tmp):
        os.remove(tmp)
    t0 = time.time()
    applied, missing_nodes, rule_hits = apply(PBF, tmp, ov)
    missing = sorted(set(ov.tags) - applied)
    empty_rules = [i for i, n in enumerate(rule_hits) if n == 0]
    if missing or missing_nodes or empty_rules:
        os.remove(tmp)
        print(f"way 없음: {missing}, add_way 노드 없음: {sorted(missing_nodes)}, 걸린 way 없는 규칙(순번): "
              f"{empty_rules} — OSM 을 다시 받았으면 {OVERRIDES} 를 다시 확인한다", file=sys.stderr)
        return 1
    os.replace(tmp, PBF)
    for r, n in zip(ov.rules, rule_hits):
        print(f"rule {r.match} name~{r.name_regex.pattern if r.name_regex else ''}: way {n}개 {r.tags}")
    for wid in sorted(applied):
        print(f"way {wid}: {ov.tags[wid]}")
    for wid, (nodes, tags) in ov.add.items():
        print(f"add_way {wid}: nodes {nodes} {tags}")
    print(f"완료 {PBF}: 규칙 {sum(rule_hits)}개·way {len(applied)}개 덮어씀, {len(ov.add)}개 추가, {time.time() - t0:.0f}초")
    return 0


if __name__ == "__main__":
    sys.exit(main())
