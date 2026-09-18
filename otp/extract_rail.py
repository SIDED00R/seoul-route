"""서울 OSM 추출본(seoul.osm.pbf)에서 철도 노선 관계(type=route, route=subway|train|light_rail)의 선로를 CSV 로 뽑는다.

사용: python otp/extract_rail.py → otp/data/rail-ways.csv (ref, way_id, seq, node_id, lat, lon)
GTFS 생성기(gtfs/internal/build/shapes_rail.go)가 이 파일로 노선별 선로 그래프를 만들어 역 사이 경로선(shapes.txt)을 낸다.
파일이 없으면 생성기는 도시철도 shape 를 쓰지 않고, OTP 는 역 사이를 직선으로 잇는다. ref 가 빈 관계(KTX·일반열차)는 뺀다.
"""

import csv
import os
import sys

import osmium

SRC = os.path.join(os.path.dirname(__file__), "data", "seoul.osm.pbf")
DST = os.path.join(os.path.dirname(__file__), "data", "rail-ways.csv")
ROUTES = ("subway", "train", "light_rail")
TRACKS = ("rail", "subway", "light_rail")  # 관계에는 승강장(railway=platform) way 도 구성원으로 들어 있다


class Relations(osmium.SimpleHandler):
    def __init__(self):
        super().__init__()
        self.refs = {}  # way_id → 그 선로를 쓰는 노선 ref 집합

    def relation(self, r):
        if r.tags.get("type") != "route" or r.tags.get("route") not in ROUTES:
            return
        ref = r.tags.get("ref", "")
        if not ref:
            return
        for m in r.members:
            if m.type == "w":
                self.refs.setdefault(m.ref, set()).add(ref)


class Ways(osmium.SimpleHandler):
    def __init__(self, refs):
        super().__init__()
        self.refs = refs
        self.rows = []

    def way(self, w):
        refs = self.refs.get(w.id)
        if not refs or w.tags.get("railway") not in TRACKS:
            return
        for ref in sorted(refs):
            for i, n in enumerate(w.nodes):
                if n.location.valid():
                    self.rows.append((ref, w.id, i, n.ref, f"{n.location.lat:.6f}", f"{n.location.lon:.6f}"))


def main() -> int:
    if not os.path.exists(SRC):
        print(f"입력 없음: {SRC}", file=sys.stderr)
        return 1
    rel = Relations()
    rel.apply_file(SRC)
    ways = Ways(rel.refs)
    ways.apply_file(SRC, locations=True)
    ways.rows.sort()
    with open(DST, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(["ref", "way_id", "seq", "node_id", "lat", "lon"])
        w.writerows(ways.rows)
    refs = {r[0] for r in ways.rows}
    tracks = {(r[0], r[1]) for r in ways.rows}
    print(f"노선 ref {len(refs)}개, 선로 {len(tracks)}개, 점 {len(ways.rows)}개 → {DST}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
