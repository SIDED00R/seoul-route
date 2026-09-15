"""서울 OSM 추출본(seoul.osm.pbf)에서 신호가 있는(있을) 횡단보도 노드를 CSV 로 뽑는다.

사용: python otp/extract_crossings.py → otp/data/crossings.csv (lat, lon, osm_id, source)
백엔드(backend/internal/crossing)가 이 파일을 읽어 도보·따릉이 구간이 지나는 신호 횡단보도마다 기대 대기를 더한다.

판정 규칙(2026-09-15 실측 근거):
- 태그로 확정: highway=crossing 노드에 crossing=traffic_signals 또는 crossing:signals=yes → 신호 있음(source=tag).
  crossing=uncontrolled/unmarked 또는 crossing:signals=no → 신호 없음(제외).
- 태그 없음(또는 crossing=marked 뿐)이면 놓인 도로 등급으로 추정(source=road): trunk/primary/secondary/tertiary 본선 위면
  신호 있음으로 본다. 근거: 태그가 있는 횡단보도만 세면 P(신호|태그) 가 primary 0.88·secondary 0.81·tertiary 0.70·
  trunk 1.00 인 반면 residential 0.19·service 0.08 이고, *_link·unclassified 는 0.35~0.6 이라 제외. 강남 일대는
  태그가 거의 없어(역삼→선릉 도보가 지나는 7개 노드 전부 무태그, primary/secondary 위) 태그만으로는 0곳이 된다.
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
