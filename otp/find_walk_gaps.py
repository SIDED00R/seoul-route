"""도보 우회로 걸린 쌍마다 그 우회를 만든 OSM 결함 후보를 찾아 효과 순으로 매긴다(도보 OSM 결함 점검 3단계, 이슈 #176).

python otp/find_walk_gaps.py [--flagged otp/data/walk-detour-flagged.json]

쌍마다 출발·도착을 감싼 영역(직선 양끝에서 max(400m, 직선×0.6) 여유)의 보행 그래프에 후보를 넣어 본다.
  R  막힌 way(foot_graph.BLOCKED: 길 성격 way 의 foot=no·access=no, highway=construction)나 보행 표시 없는
     자전거길(foot_graph.CYCLE_ONLY)을 다시 연다
  L  막다른 길 끝(foot_graph.FootGraph.is_dead_end)에서 LINK_MAX 안의 다른 길 노드로 잇는다
후보 없이도 OTP 보다 훨씬 짧으면 "model"(OTP 쪽 제약 — private 통과 제한·비용 차이), 후보를 다 넣으면 짧아지면
"fixable"(후보를 하나씩 빼 보며 꼭 필요한 최소 묶음을 남긴다), 둘 다 아니면 "unexplained"(하천·철도 같은 실제 장벽이거나
영역 밖 문제). flagged 에 TMap 경로선(tmap_line)이 있으면 후보가 그 선에서 TMAP_NEAR 안인지도 적는다.

출력: data/walk-gaps.json(후보 전부와 오버라이드 초안), docs/eval/walk-gaps-<날짜>.md(요약).
같은 서울 보행 그래프를 foot_graph 로 읽으므로 patch_osm.py 를 적용한 pbf 면 이미 고친 곳은 다시 나오지 않는다.
"""

import argparse
import datetime
import json
import math
import os
import sys
import time

from dataclasses import dataclass

import foot_graph as fg
import link_barriers as lb

DIR = os.path.dirname(os.path.abspath(__file__))
DATA = os.path.join(DIR, "data")
LINK_MAX = 60.0  # 막다른 길 끝에서 이을 다른 길을 찾는 반경(m). 양평 보도육교 틈 21~50m 를 담는 값
SNAP_MAX = 150.0  # 출발·도착을 보행 그래프 노드에 붙이는 반경(m)
SHRINK = 0.75  # 후보로 줄어든 거리가 OTP 의 이 비율 이하이고
MIN_SAVING = 300.0  # 줄어든 양이 이 이상이면 설명된 것으로 본다
KEEP_SLACK = 150.0  # 최소 묶음을 고를 때 빼도 이만큼까지 늘어나는 후보는 뺀다
TMAP_NEAR = 25.0  # 후보가 TMap 경로선에서 이 거리 안이면 TMap 도 그 길로 간다고 본다
# 막다른 끝을 잇지 않을 길: 진출입 램프·간선도로는 보도가 없는 경우가 많다(양평 검증에서 램프로 잇는 후보가 나왔다)
NO_LINK_HIGHWAY = {"trunk", "trunk_link", "primary_link", "secondary_link", "tertiary_link"}


def pair_bbox(f: dict) -> tuple[float, float, float, float]:
    buf = max(400.0, f["straight_m"] * 0.6)
    dlat, dlon = buf / 111_000, buf / (111_000 * math.cos(math.radians(f["o_lat"])))
    return (min(f["o_lon"], f["d_lon"]) - dlon, min(f["o_lat"], f["d_lat"]) - dlat,
            max(f["o_lon"], f["d_lon"]) + dlon, max(f["o_lat"], f["d_lat"]) + dlat)


@dataclass
class Ctx:
    g: fg.FootGraph
    dead_idx: dict[tuple[int, int], list[int]]
    blocked: list[fg.Way]
    barriers: lb.BarrierIndex


def make_ctx(g: fg.FootGraph, barriers: lb.BarrierIndex) -> Ctx:
    idx: dict[tuple[int, int], list[int]] = {}
    for nid in g.adj:
        if g.is_dead_end(nid):
            lat, lon = g.coords[nid]
            idx.setdefault((int(lat / fg.CELL), int(lon / fg.CELL)), []).append(nid)
    return Ctx(g, idx, [w for w in g.ways.values() if w.cls in fg.CANDIDATE], barriers)


def inside_structure(g: fg.FootGraph, nid: int) -> bool:
    """nid 가 다리·터널 way 의 끝이 아닌 중간 노드인지 — 땅 위 길과 높이가 달라 이을 수 없다."""
    for wid in g.node_ways.get(nid, ()):
        w = g.ways[wid]
        structure = w.tags.get("bridge", "no") != "no" or w.tags.get("tunnel", "no") != "no"
        if structure and nid not in (w.nodes[0], w.nodes[-1]):
            return True
    return False


def candidates(ctx: Ctx, bbox) -> dict[tuple, list[tuple[int, int, float]]]:
    """후보 표지 → 간선 [(a, b, 길이)…]. R 은 ("R", way id), L 은 ("L", 막다른 끝, 이을 노드).

    L 은 램프·간선도로(NO_LINK_HIGHWAY)와 다리·터널 중간으로 잇지 않고, 담장·철로·물·고속도로(link_barriers.HARD)를
    가로지르는 것도 뺀다.
    """
    g = ctx.g
    out: dict[tuple, list[tuple[int, int, float]]] = {}
    for w in ctx.blocked:
        if any(fg.in_bbox(g.coords[n], bbox) for n in w.nodes):
            out[("R", w.id)] = [(a, b, fg.haversine(*g.coords[a], *g.coords[b]))
                                for a, b in zip(w.nodes, w.nodes[1:]) if a != b]
    for i in range(int(bbox[1] / fg.CELL), int(bbox[3] / fg.CELL) + 1):
        for j in range(int(bbox[0] / fg.CELL), int(bbox[2] / fg.CELL) + 1):
            for a in ctx.dead_idx.get((i, j), ()):
                if not fg.in_bbox(g.coords[a], bbox):
                    continue
                own = set(g.node_ways[a])
                if any(g.ways[w].tags.get("highway") in NO_LINK_HIGHWAY for w in own):
                    continue
                seen_ways: set[int] = set()
                for d, b in g.near(*g.coords[a], LINK_MAX):
                    bw = {w for w in g.node_ways.get(b, ()) if w not in own
                          and g.ways[w].tags.get("highway") not in NO_LINK_HIGHWAY}
                    if b == a or not bw or bw & seen_ways or any(v == b for v, _, _ in g.adj.get(a, ())):
                        continue
                    if inside_structure(g, b) or lb.HARD in ctx.barriers.crossings(g.coords[a], g.coords[b], own | bw):
                        continue
                    seen_ways |= bw
                    out[("L", a, b)] = [(a, b, d)]
                    if len(seen_ways) >= 3:
                        break
    return out


def as_extra(cands: dict[tuple, list[tuple[int, int, float]]], keys) -> fg.Extra:
    extra: fg.Extra = {}
    for k in keys:
        for a, b, d in cands[k]:
            extra.setdefault(a, []).append((b, d, k))
            extra.setdefault(b, []).append((a, d, k))
    return extra


def explain(ctx: Ctx, f: dict) -> dict:
    """쌍 하나의 판정. category·local0_m·fixed_m·set(최소 후보 묶음)·private(지난 private way)."""
    g = ctx.g
    bbox = pair_bbox(f)
    so = g.near(f["o_lat"], f["o_lon"], SNAP_MAX)
    sd = g.near(f["d_lat"], f["d_lon"], SNAP_MAX)
    res = {"category": "unexplained", "local0_m": None, "fixed_m": None, "set": [], "private": []}
    if not so or not sd:
        res["category"] = "snap"
        return res
    (do, src), (dd, dst) = so[0], sd[0]
    snap = do + dd
    otp = f["otp_m"]
    d0, used0 = fg.shortest(g, src, dst, bbox)
    res["local0_m"] = round(d0 + snap) if d0 < math.inf else None
    if d0 + snap <= otp * SHRINK and otp - (d0 + snap) >= MIN_SAVING:
        res["category"] = "model"
        res["private"] = [t[1] for t in used0 if t[0] == "P"]
        return res
    cands = candidates(ctx, bbox)
    dall, used = fg.shortest(g, src, dst, bbox, as_extra(cands, cands))
    if not (dall + snap <= otp * SHRINK and otp - (dall + snap) >= MIN_SAVING):
        return res
    keep = [t for t in used if t[0] in ("R", "L")]
    for k in list(keep):
        trial = [x for x in keep if x != k]
        dk, _ = fg.shortest(g, src, dst, bbox, as_extra(cands, trial))
        if dk <= dall + KEEP_SLACK:
            keep = trial
    dset, _ = fg.shortest(g, src, dst, bbox, as_extra(cands, keep))
    res.update(category="fixable", fixed_m=round(dset + snap), set=keep)
    return res


def point_line_m(lat: float, lon: float, line: list[tuple[float, float]]) -> float:
    """(lat, lon) 에서 경로선(꼭짓점 목록)까지 최소 거리(m). 선분까지 잰다 — TMap 선은 직선 구간에 꼭짓점이 드물다
    (양평 나들목→테니스장 805m 에 37개)."""
    if not line:
        return math.inf
    kx = 111_320 * math.cos(math.radians(lat))  # 위경도 → 이 점 기준 평면 m
    ky = 110_540
    best = math.inf
    pts = [((lo - lon) * kx, (la - lat) * ky) for la, lo in line]
    if len(pts) == 1:
        return math.hypot(*pts[0])
    for (ax, ay), (bx, by) in zip(pts, pts[1:]):
        dx, dy = bx - ax, by - ay
        t = 0.0 if dx == dy == 0 else max(0.0, min(1.0, -(ax * dx + ay * dy) / (dx * dx + dy * dy)))
        best = min(best, math.hypot(ax + t * dx, ay + t * dy))
    return best


TMAP_SAMPLE = 5.0  # 막힌 way 를 TMap 선과 대조할 때 way 위에 찍는 점 간격(m)
TMAP_MIN_OVERLAP = 30.0  # 막힌 way 중 TMap 선에 TMAP_NEAR 안으로 겹치는 길이가 이것(또는 way 길이의 절반) 이상이면 지난다고 본다


def on_tmap(ctx: Ctx, key: tuple, line: list[tuple[float, float]]) -> bool:
    """후보가 TMap 경로선 위에 있는지. L 은 양끝이 모두 TMAP_NEAR 안, R 은 way 를 TMAP_SAMPLE 간격으로 찍은 점 중
    TMAP_NEAR 안인 점의 길이 합이 min(TMAP_MIN_OVERLAP, way 길이 절반) 이상."""
    g = ctx.g
    if key[0] == "L":
        return all(point_line_m(*g.coords[n], line) <= TMAP_NEAR for n in key[1:])
    w = g.ways[key[1]]
    near = 0.0
    for a, b in zip(w.nodes, w.nodes[1:]):
        (la1, lo1), (la2, lo2) = g.coords[a], g.coords[b]
        n = max(1, int(fg.haversine(la1, lo1, la2, lo2) / TMAP_SAMPLE))
        for i in range(n):
            t = (i + 0.5) / n
            if point_line_m(la1 + (la2 - la1) * t, lo1 + (lo2 - lo1) * t, line) <= TMAP_NEAR:
                near += fg.haversine(la1, lo1, la2, lo2) / n
    return near >= min(TMAP_MIN_OVERLAP, w.length_m / 2)


SHORT_LINK = 20.0  # 이보다 짧고 큰길을 가로지르지 않는 L 은 등급 A(지도 그릴 때 덜 이은 것일 가능성이 크다)


def describe(ctx: Ctx, key: tuple) -> dict:
    """후보 설명. grade: R = 사람이 확인(폐쇄가 아직 유효할 수 있다), A = 짧고 큰길을 안 건너는 L, B = 그 밖의 L."""
    g = ctx.g
    if key[0] == "R":
        w = g.ways[key[1]]
        mid = g.coords[w.nodes[len(w.nodes) // 2]]
        return {"type": "R", "grade": "R", "way": w.id, "cycle_only": w.cls == fg.CYCLE_ONLY,
                "highway": w.tags.get("highway", ""),
                "name": w.tags.get("name", ""), "foot": w.tags.get("foot", ""), "access": w.tags.get("access", ""),
                "edited": w.timestamp, "length_m": round(w.length_m), "lat": round(mid[0], 6), "lon": round(mid[1], 6),
                "override": {"way": w.id, "tags": {"foot": "yes"}}}
    a, b = key[1], key[2]
    pa, pb = g.coords[a], g.coords[b]
    length = fg.haversine(*pa, *pb)
    road = lb.ROAD in ctx.barriers.crossings(pa, pb, set(g.node_ways[a]) | set(g.node_ways.get(b, ())))
    return {"type": "L", "grade": "A" if length <= SHORT_LINK and not road else "B", "crosses_road": road,
            "from_node": a, "to_node": b, "length_m": round(length),
            "from_way": g.node_ways[a][0], "to_ways": g.node_ways.get(b, []),
            "lat": round((pa[0] + pb[0]) / 2, 6), "lon": round((pa[1] + pb[1]) / 2, 6),
            "override": {"add_way": None, "nodes": [a, b], "tags": {"highway": "footway"}}}


def aggregate(ctx: Ctx, flagged: list[dict], results: list[dict]) -> list[dict]:
    agg: dict[tuple, dict] = {}
    for f, r in zip(flagged, results):
        if r["category"] != "fixable":
            continue
        saving = f["otp_m"] - r["fixed_m"]
        for k in r["set"]:
            c = agg.setdefault(k, describe(ctx, k) | {"pairs": 0, "best_saving_m": 0, "solo": 0, "tmap_on": 0,
                                                    "tmap_off": 0, "example": None})
            c["pairs"] += 1
            if len(r["set"]) == 1:
                c["solo"] += 1
            if saving > c["best_saving_m"]:
                c["best_saving_m"] = round(saving)
                c["example"] = {"o": [f["o_lat"], f["o_lon"], f["o_name"]], "d": [f["d_lat"], f["d_lon"], f["d_name"]],
                                "otp_m": round(f["otp_m"]), "fixed_m": r["fixed_m"], "set_size": len(r["set"])}
            if f.get("tmap_line"):
                c["tmap_on" if on_tmap(ctx, k, f["tmap_line"]) else "tmap_off"] += 1
    return sorted(agg.values(), key=lambda c: (-c["pairs"] * c["best_saving_m"], c["length_m"]))


def write_md(path: str, flagged: list[dict], results: list[dict], cands: list[dict], top: int = 60) -> None:
    cnt: dict[str, int] = {}
    for r in results:
        cnt[r["category"]] = cnt.get(r["category"], 0) + 1
    priv: dict[int, int] = {}
    for r in results:
        for w in r["private"]:
            priv[w] = priv.get(w, 0) + 1
    lines = [f"# 도보 끊김 후보 ({datetime.date.today().isoformat()})", "",
             f"`otp/find_walk_gaps.py` 결과. 입력 = 도보 우회로 걸린 쌍 {len(flagged)}건.", "",
             "| 판정 | 쌍 | 뜻 |", "|---|---|---|",
             f"| fixable | {cnt.get('fixable', 0)} | 막힌 way 다시 열기(R)·막다른 끝 잇기(L) 후보로 설명된다 |",
             f"| model | {cnt.get('model', 0)} | 후보 없이도 OTP 보다 짧다 — OTP 쪽 제약(보행 안전 가중치·private 통과 제한) |",
             f"| unexplained | {cnt.get('unexplained', 0)} | 하천·철도 같은 실제 장벽이거나 영역 밖 문제 |",
             f"| snap | {cnt.get('snap', 0)} | 출발·도착 {SNAP_MAX:.0f}m 안에 보행 길이 없다 |",
             "", f"## 후보 (쌍 수 × 최대 절감 순, 최대 {top}건)", "",
             "`단독` = 그 후보 하나로 설명된 쌍 수. TMap 열은 걸린 쌍에 TMap 경로선이 있을 때만 찬다(지남/안 지남).",
             f"등급: R = 막힌 way 다시 열기(사람 확인), A = {SHORT_LINK:.0f}m 이하이고 큰길을 안 건너는 잇기, B = 그 밖의 잇기.",
             "",
             "| 등급 | OSM | 이름·태그 | 길이(m) | 편집일 | 쌍 | 단독 | 최대 절감(m) | TMap | 위치 |",
             "|---|---|---|---|---|---|---|---|---|---|"]
    for c in cands[:top]:
        if c["type"] == "R":
            osm = f"[way {c['way']}](https://www.openstreetmap.org/way/{c['way']})"
            tag = f"{c['highway']} {c['name']} foot={c['foot'] or '-'} access={c['access'] or '-'}"
            if c["cycle_only"]:
                tag += " (보행 표시 없는 자전거길)"
        else:
            osm = (f"[node {c['from_node']}](https://www.openstreetmap.org/node/{c['from_node']}) → "
                   f"[node {c['to_node']}](https://www.openstreetmap.org/node/{c['to_node']})")
            tag = f"way {c['from_way']} 끝 → way {','.join(map(str, c['to_ways']))}"
            if c["crosses_road"]:
                tag += " (큰길 가로지름)"
        tm = f"{c['tmap_on']}/{c['tmap_off']}" if c["tmap_on"] or c["tmap_off"] else "-"
        lines.append(f"| {c['grade']} | {osm} | {tag} | {c['length_m']} | {c.get('edited', '')} | {c['pairs']} | "
                     f"{c['solo']} | {c['best_saving_m']} | {tm} | {c['lat']},{c['lon']} |")
    if priv:
        lines += ["", "## model 판정 쌍이 지난 private way (많은 순, 최대 20건)", "",
                  "| way | 쌍 |", "|---|---|"]
        lines += [f"| [{w}](https://www.openstreetmap.org/way/{w}) | {n} |"
                  for w, n in sorted(priv.items(), key=lambda kv: -kv[1])[:20]]
    with open(path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines) + "\n")


def main() -> int:
    today = datetime.date.today().isoformat()
    ap = argparse.ArgumentParser()
    ap.add_argument("--pbf", default=os.path.join(DATA, "seoul.osm.pbf"))
    ap.add_argument("--flagged", default=os.path.join(DATA, "walk-detour-flagged.json"))
    ap.add_argument("--out-json", default=os.path.join(DATA, "walk-gaps.json"))
    ap.add_argument("--out-md", default=os.path.normpath(os.path.join(DIR, "..", "docs", "eval",
                                                                      f"walk-gaps-{today}.md")))
    a = ap.parse_args()
    with open(a.flagged, encoding="utf-8") as fh:
        flagged = json.load(fh)
    t0 = time.time()
    ctx = make_ctx(fg.load(a.pbf), lb.load(a.pbf))
    print(f"그래프·장벽 {time.time() - t0:.0f}초, 막다른 끝 {sum(map(len, ctx.dead_idx.values()))}, "
          f"막힌 way {len(ctx.blocked)}")
    t0 = time.time()
    results = []
    for i, f in enumerate(flagged):
        results.append(explain(ctx, f))
        if (i + 1) % 100 == 0:
            print(f"  {i + 1}/{len(flagged)} {time.time() - t0:.0f}초")
    cands = aggregate(ctx, flagged, results)
    pairs_out = [{k: v for k, v in (f | r).items() if k != "tmap_line"} | {
        "set": [list(k) for k in r["set"]]} for f, r in zip(flagged, results)]
    with open(a.out_json, "w", encoding="utf-8") as fh:
        json.dump({"candidates": cands, "pairs": pairs_out}, fh, ensure_ascii=False, indent=1)
    write_md(a.out_md, flagged, results, cands)
    print(f"판정 {time.time() - t0:.0f}초, 후보 {len(cands)} → {a.out_md}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
