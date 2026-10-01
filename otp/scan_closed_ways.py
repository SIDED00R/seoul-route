"""서울 보행 길 중 폐쇄 표시가 붙은 way 를 목록으로 만든다(도보 OSM 결함 점검 1단계, 이슈 #176).

python otp/scan_closed_ways.py [--out-csv otp/data/osm-closed-ways.csv] [--out-md docs/eval/osm-closed-ways-<날짜>.md]

잡는 것:
  - 길 성격 way(foot_graph.PATHLIKE_HIGHWAY)의 foot=no·access=no, highway=construction (foot_graph.BLOCKED)
  - 걷는 way 든 막힌 way 든 이름·메모에 공사·폐쇄·통제 같은 말이 있는 것
편집일(OSM timestamp)이 오래된 폐쇄 표시는 공사가 끝났는데 안 지운 것일 수 있다. 실제로 막는지는 find_walk_gaps 가
도보 우회와 겹치는지로 가린다.
"""

import argparse
import csv
import datetime
import os
import re
import sys

import foot_graph as fg

DIR = os.path.dirname(os.path.abspath(__file__))
PBF = os.path.join(DIR, "data", "seoul.osm.pbf")
TEXT_KEYS = ("name", "name:ko", "note", "description", "fixme")
KEYWORD = re.compile(r"공사|폐쇄|통제|임시|철거|closed|construction|under construction", re.IGNORECASE)


def reasons(w: fg.Way) -> list[str]:
    out = []
    if w.cls == fg.BLOCKED:
        h = w.tags.get("highway")
        if h == "construction":
            out.append("highway=construction")
        elif w.tags.get("foot") == "no":
            out.append("foot=no")
        else:
            out.append(f"access={w.tags.get('access')}")
    for k in TEXT_KEYS:
        v = w.tags.get(k, "")
        m = KEYWORD.search(v) if v else None
        if m:
            out.append(f"{k}~{m.group(0)}")
    return out


def scan(g: fg.FootGraph) -> list[dict]:
    rows = []
    for w in g.ways.values():
        rs = reasons(w)
        if not rs:
            continue
        mid = g.coords[w.nodes[len(w.nodes) // 2]]
        rows.append({
            "way": w.id, "class": w.cls, "highway": w.tags.get("highway", ""), "name": w.tags.get("name", ""),
            "reasons": ";".join(rs), "edited": w.timestamp, "length_m": round(w.length_m),
            "lat": round(mid[0], 6), "lon": round(mid[1], 6),
        })
    # 이름에 폐쇄 표시가 있는 것 먼저, 그 안에서 편집이 오래된 순
    rows.sort(key=lambda r: ("~" not in r["reasons"], r["edited"] or "9999", -r["length_m"]))
    return rows


def write_csv(rows: list[dict], path: str) -> None:
    with open(path, "w", encoding="utf-8", newline="") as f:
        wr = csv.DictWriter(f, fieldnames=list(rows[0]) if rows else ["way"])
        wr.writeheader()
        wr.writerows(rows)


def write_md(rows: list[dict], path: str, top: int = 60) -> None:
    by_reason: dict[str, int] = {}
    for r in rows:
        for x in r["reasons"].split(";"):
            key = x.split("~")[0] + ("~" if "~" in x else "")
            by_reason[key] = by_reason.get(key, 0) + 1
    lines = [f"# OSM 보행 폐쇄 표시 조사 ({datetime.date.today().isoformat()})", "",
             f"`otp/scan_closed_ways.py` 결과. 전체 {len(rows)}건(way 기준, 한 way 가 여러 사유에 걸릴 수 있다).", "",
             "| 사유 | 건수 |", "|---|---|"]
    lines += [f"| `{k}` | {v} |" for k, v in sorted(by_reason.items(), key=lambda kv: -kv[1])]
    lines += ["", f"## 이름·메모에 폐쇄 표시가 있는 way (오래된 편집 순, 최대 {top}건)", "",
              "| way | 분류 | highway | 이름 | 사유 | 편집일 | 길이(m) | 위치 |", "|---|---|---|---|---|---|---|---|"]
    named = [r for r in rows if "~" in r["reasons"]][:top]
    lines += [f"| [{r['way']}](https://www.openstreetmap.org/way/{r['way']}) | {r['class']} | {r['highway']} | "
              f"{r['name']} | {r['reasons']} | {r['edited']} | {r['length_m']} | {r['lat']},{r['lon']} |"
              for r in named]
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")


def main() -> int:
    today = datetime.date.today().isoformat()
    ap = argparse.ArgumentParser()
    ap.add_argument("--pbf", default=PBF)
    ap.add_argument("--out-csv", default=os.path.join(DIR, "data", "osm-closed-ways.csv"))
    ap.add_argument("--out-md", default=os.path.normpath(os.path.join(DIR, "..", "docs", "eval",
                                                                      f"osm-closed-ways-{today}.md")))
    a = ap.parse_args()
    if not os.path.exists(a.pbf):
        print(f"입력 없음: {a.pbf}", file=sys.stderr)
        return 1
    rows = scan(fg.load(a.pbf))
    write_csv(rows, a.out_csv)
    write_md(rows, a.out_md)
    print(f"{len(rows)}건 → {a.out_csv}, {a.out_md}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
