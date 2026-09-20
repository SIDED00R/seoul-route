"""서울 OSM에서 신호 횡단보도 노드를 crossings.csv로 추출한다.

명시적 신호 태그를 우선하고, 태그가 없으면 간선도로 등급으로 추정한다.
"""

import csv
import os
import sys

import osmium

SRC = os.path.join(os.path.dirname(__file__), "data", "seoul.osm.pbf")
DST = os.path.join(os.path.dirname(__file__), "data", "crossings.csv")
MAJOR = {"trunk", "primary", "secondary", "tertiary"}


class Nodes(osmium.SimpleHandler):
    def __init__(self):
        super().__init__()
        self.tagged = {}  # id -> (lat, lon)  신호 태그 확정
        self.unknown = {}  # id -> (lat, lon)  태그 없음(도로 등급으로 판정)
        self.total = 0

    def node(self, n):
        if n.tags.get("highway") != "crossing":
            return
        self.total += 1
        t, s = n.tags.get("crossing", ""), n.tags.get("crossing:signals", "")
        loc = (n.location.lat, n.location.lon)
        if t == "traffic_signals" or s == "yes":
            self.tagged[n.id] = loc
        elif t in ("uncontrolled", "unmarked") or s == "no":
            return
        else:
            self.unknown[n.id] = loc


class Ways(osmium.SimpleHandler):
    def __init__(self, unknown):
        super().__init__()
        self.unknown = unknown
        self.on_major = set()

    def way(self, w):
        if w.tags.get("highway") not in MAJOR:
            return
        for nd in w.nodes:
            if nd.ref in self.unknown:
                self.on_major.add(nd.ref)


def main() -> int:
    if not os.path.exists(SRC):
        print(f"입력 없음: {SRC}", file=sys.stderr)
        return 1
    nodes = Nodes()
    nodes.apply_file(SRC)
    ways = Ways(nodes.unknown)
    ways.apply_file(SRC)
    rows = [(f"{la:.6f}", f"{lo:.6f}", i, "tag") for i, (la, lo) in nodes.tagged.items()]
    rows += [(f"{la:.6f}", f"{lo:.6f}", i, "road") for i, (la, lo) in nodes.unknown.items() if i in ways.on_major]
    rows.sort()
    with open(DST, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(["lat", "lon", "osm_id", "source"])
        w.writerows(rows)
    print(f"횡단보도 노드 {nodes.total}개: 신호 태그 {len(nodes.tagged)}, 무태그 {len(nodes.unknown)} 중 간선 위 "
          f"{len(ways.on_major)} → 신호 횡단보도 {len(rows)}개 → {DST}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
