"""Geofabrik south-korea pbf에서 서울 bbox만 잘라 seoul.osm.pbf 를 만든다.

bbox는 서울시 행정경계(126.764~127.184E, 37.428~37.701N)에 약 4km 여유를 둔 값이다.
경계 밖 정류장·대여소가 그래프에 연결되도록 여유를 둔다.

pyosmium 4.x 에는 bbox 필터가 없어 3패스로 직접 자른다(osmium extract 의 complete_ways 전략).
  1) bbox 안 노드 id 수집
  2) 그 노드를 하나라도 쓰는 way 를 통째로 채택하고, way 의 모든 노드를 필요 노드에 추가
  3) 채택된 노드·way 를 멤버로 갖는 relation 채택 (relation 은 재귀하지 않음)
"""

import os
import sys
import time

import osmium

SRC = os.path.join(os.path.dirname(__file__), "data", "south-korea-latest.osm.pbf")
DST = os.path.join(os.path.dirname(__file__), "data", "seoul.osm.pbf")
MIN_LON, MIN_LAT, MAX_LON, MAX_LAT = 126.70, 37.38, 127.25, 37.75


def log(msg: str) -> None:
    print(f"[{time.strftime('%H:%M:%S')}] {msg}", flush=True)


def main() -> int:
    if not os.path.exists(SRC):
        print(f"입력 없음: {SRC}", file=sys.stderr)
        return 1
    if os.path.exists(DST):
        os.remove(DST)

    keep_nodes: set[int] = set()
    for n in osmium.FileProcessor(SRC, osmium.osm.NODE):
        loc = n.location
        if loc.valid() and MIN_LON <= loc.lon <= MAX_LON and MIN_LAT <= loc.lat <= MAX_LAT:
            keep_nodes.add(n.id)
    log(f"1/4 bbox 노드 {len(keep_nodes):,}")

    keep_ways: set[int] = set()
    need_nodes: set[int] = set()
    for w in osmium.FileProcessor(SRC, osmium.osm.WAY):
        refs = [nr.ref for nr in w.nodes]
        if any(r in keep_nodes for r in refs):
            keep_ways.add(w.id)
            need_nodes.update(refs)
    keep_nodes |= need_nodes
    log(f"2/4 way {len(keep_ways):,}, 노드 합계 {len(keep_nodes):,}")

    keep_rels: set[int] = set()
    for r in osmium.FileProcessor(SRC, osmium.osm.RELATION):
        for m in r.members:
            if (m.type == "n" and m.ref in keep_nodes) or (m.type == "w" and m.ref in keep_ways):
                keep_rels.add(r.id)
                break
    log(f"3/4 relation {len(keep_rels):,}")

    written = {"node": 0, "way": 0, "relation": 0}
    with osmium.SimpleWriter(DST) as writer:
        for o in osmium.FileProcessor(SRC):
            if o.is_node():
                t, keep = "node", o.id in keep_nodes
            elif o.is_way():
                t, keep = "way", o.id in keep_ways
            else:
                t, keep = "relation", o.id in keep_rels
            if keep:
                writer.add(o)
                written[t] += 1
    log(f"4/4 완료 {DST}: {written}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
