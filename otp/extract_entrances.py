"""서울 OSM 추출본(seoul.osm.pbf)에서 지하철 출입구(railway=subway_entrance) 노드를 CSV 로 뽑는다.

사용: python otp/extract_entrances.py → otp/data/subway-entrances.csv (lat, lon, ref, name)
GTFS 생성기(gtfs/internal/build/pathways.go)가 이 파일을 읽어 부모역 반경 안 출입구를 location_type=2 로 쓴다.
파일이 없으면 생성기는 승강장 좌표에 출입구를 두는 이전 방식으로 돌아간다. 2026-09-13 실측 2,457개(공사 중 3개 제외, ref 있음 2,434).
"""

import csv
import os
import sys

import osmium

SRC = os.path.join(os.path.dirname(__file__), "data", "seoul.osm.pbf")
DST = os.path.join(os.path.dirname(__file__), "data", "subway-entrances.csv")


class Handler(osmium.SimpleHandler):
    def __init__(self):
        super().__init__()
        self.rows = []

    def node(self, n):
        if n.tags.get("railway") != "subway_entrance":
            return
        # 공사 중 출입구 제외. 2026-09-13 데이터에 access/disused/opening_date 류 태그는 0건이고 공사 표식은
        # construction:railway 1건(청량리 5번), ref "현재 공사중" 1건(시흥대야 4번), description "(공사중)" 1건(굴포천 7번)뿐.
        if "construction:railway" in n.tags:
            return
        if any("공사" in n.tags.get(k, "") for k in ("ref", "name", "description", "description:ko")):
            return
        self.rows.append(
            (f"{n.location.lat:.6f}", f"{n.location.lon:.6f}", n.tags.get("ref", ""), n.tags.get("name", "")))


def main() -> int:
    if not os.path.exists(SRC):
        print(f"입력 없음: {SRC}", file=sys.stderr)
        return 1
    h = Handler()
    h.apply_file(SRC)
    h.rows.sort()
    with open(DST, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(["lat", "lon", "ref", "name"])
        w.writerows(h.rows)
    print(f"출입구 {len(h.rows)}개 → {DST}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
