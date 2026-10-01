"""도보가 비정상으로 도는 출발·도착 쌍을 고른다(도보 OSM 결함 점검 2단계, 이슈 #176).

python otp/walk_detour_scan.py [--otp http://127.0.0.1:8083] [--per-origin 4] [--seed 1]

표본: 출발 = 지하철 출입구(data/subway-entrances.csv)·따릉이 대여소(백엔드 GBFS), 도착 = 출발에서 직선 250~1000m 안의
상가(data/shop-places-seoul.csv)와 OSM 시설(공원·운동장·학교 등). 출발마다 도착을 per-origin 개 뽑아 OTP 에 도보만
질의하고, OTP 거리 ÷ 직선거리 ≥ ratio 이고 초과분 ≥ excess 인 쌍을 걸러낸다.
TMAP_APP_KEY(환경변수, 없으면 레포 루트 .env)가 있으면 걸린 쌍만 TMap 보행자 경로(apis.openapi.sk.com)로 다시 재
대조한다(--tmap-limit 건까지).

출력: data/walk-detour-pairs.csv(전체), data/walk-detour-flagged.json(걸린 쌍 — find_walk_gaps 입력),
docs/eval/walk-detour-<날짜>.md(요약).
OTP 주소는 127.0.0.1 로 준다 — 윈도우 파이썬은 localhost 를 IPv6 로 먼저 시도해 질의마다 2초씩 늦다(2026-10-01 실측).
"""

import argparse
import csv
import datetime
import json
import os
import random
import sys
import time
import urllib.request
from concurrent.futures import ThreadPoolExecutor

import osmium

from foot_graph import haversine

DIR = os.path.dirname(os.path.abspath(__file__))
DATA = os.path.join(DIR, "data")
POI_KEYS = {
    "leisure": {"park", "pitch", "sports_centre", "playground", "garden", "stadium", "track", "fitness_station"},
    "amenity": {"school", "university", "college", "hospital", "library", "community_centre", "townhall",
                "place_of_worship", "theatre", "cinema", "marketplace", "police", "post_office"},
    "tourism": {"attraction", "museum", "gallery", "viewpoint", "zoo"},
}
TMAP_URL = "https://apis.openapi.sk.com/tmap/routes/pedestrian?version=1&format=json"
WALK_QUERY = """query($o: PlanLabeledLocationInput!, $d: PlanLabeledLocationInput!) {
 planConnection(origin: $o, destination: $d, modes: {direct: [WALK], directOnly: true}, first: 1) {
  edges { node { walkDistance } } } }"""


def load_origins(gbfs_url: str) -> list[dict]:
    out = []
    with open(os.path.join(DATA, "subway-entrances.csv"), encoding="utf-8") as f:
        for r in csv.DictReader(f):
            out.append({"lat": float(r["lat"]), "lon": float(r["lon"]), "name": f"출입구 {r['ref']} {r['name']}".strip()})
    try:
        with urllib.request.urlopen(gbfs_url, timeout=30) as r:
            for s in json.load(r)["data"]["stations"]:
                out.append({"lat": s["lat"], "lon": s["lon"], "name": f"따릉이 {s['name']}"})
    except OSError as e:
        print(f"GBFS 못 읽음({e}) — 지하철 출입구만 쓴다", file=sys.stderr)
    return out


def load_shops() -> list[dict]:
    out = []
    with open(os.path.join(DATA, "shop-places-seoul.csv"), encoding="utf-8") as f:
        for r in csv.DictReader(f):
            try:
                out.append({"lat": float(r["lat"]), "lon": float(r["lon"]), "name": r["name"], "kind": "shop"})
            except ValueError:
                continue
    return out


def poi_kind(tags) -> str:
    for k, vs in POI_KEYS.items():
        v = tags.get(k)
        if v in vs:
            return f"{k}={v}"
    return ""


def load_osm_pois(pbf: str) -> list[dict]:
    """POI_KEYS 에 드는 노드와 way(노드 좌표 평균)."""
    out = []
    for o in osmium.FileProcessor(pbf).with_locations():
        if o.is_relation():
            continue
        kind = poi_kind(o.tags)
        if not kind:
            continue
        if o.is_node():
            lat, lon = o.location.lat, o.location.lon
        else:
            locs = [(n.location.lat, n.location.lon) for n in o.nodes if n.location.valid()]
            if not locs:
                continue
            lat, lon = sum(x[0] for x in locs) / len(locs), sum(x[1] for x in locs) / len(locs)
        out.append({"lat": lat, "lon": lon, "name": o.tags.get("name", ""), "kind": kind})
    return out


CELL = 0.005  # 약 500m


def grid_index(points: list[dict]) -> dict[tuple[int, int], list[int]]:
    g: dict[tuple[int, int], list[int]] = {}
    for i, p in enumerate(points):
        g.setdefault((int(p["lat"] / CELL), int(p["lon"] / CELL)), []).append(i)
    return g


def sample_pairs(origins: list[dict], dests: list[dict], per_origin: int, min_m: float, max_m: float,
                 rng: random.Random) -> list[tuple[dict, dict, float]]:
    """출발마다 직선 min_m~max_m 안 도착을 per_origin 개(없으면 있는 만큼) 뽑는다."""
    idx = grid_index(dests)
    k = int(max_m / 440) + 1
    pairs = []
    for o in origins:
        ci, cj = int(o["lat"] / CELL), int(o["lon"] / CELL)
        cand = []
        for i in range(ci - k, ci + k + 1):
            for j in range(cj - k, cj + k + 1):
                for di in idx.get((i, j), ()):
                    d = haversine(o["lat"], o["lon"], dests[di]["lat"], dests[di]["lon"])
                    if min_m <= d <= max_m:
                        cand.append((di, d))
        for di, d in rng.sample(cand, min(per_origin, len(cand))):
            pairs.append((o, dests[di], d))
    return pairs


def otp_walk(otp: str, o: dict, d: dict) -> float | None:
    v = {"o": {"location": {"coordinate": {"latitude": o["lat"], "longitude": o["lon"]}}},
         "d": {"location": {"coordinate": {"latitude": d["lat"], "longitude": d["lon"]}}}}
    req = urllib.request.Request(f"{otp}/otp/gtfs/v1", json.dumps({"query": WALK_QUERY, "variables": v}).encode(),
                                 {"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=60) as r:
        edges = json.load(r)["data"]["planConnection"]["edges"]
    return edges[0]["node"]["walkDistance"] if edges else None


def dotenv_value(path: str, key: str) -> str:
    """.env 에서 key 값. 백엔드 config.readDotEnv 와 같게 첫 '=' 에서 나누고 양끝 같은 따옴표를 벗긴다. 없으면 빈 값."""
    try:
        with open(path, encoding="utf-8") as f:
            lines = f.readlines()
    except OSError:
        return ""
    for line in lines:
        t = line.strip()
        if not t or t.startswith("#") or "=" not in t:
            continue
        k, v = t.split("=", 1)
        if k.strip() != key:
            continue
        v = v.strip()
        if len(v) >= 2 and v[0] == v[-1] and v[0] in "'\"":
            v = v[1:-1]
        return v
    return ""


def tmap_key() -> str:
    """환경변수가 비어 있지 않으면 그 값, 아니면 레포 루트 .env 의 값(백엔드 config.Load 와 같은 순서)."""
    return os.environ.get("TMAP_APP_KEY", "") or dotenv_value(os.path.join(os.path.dirname(DIR), ".env"),
                                                               "TMAP_APP_KEY")


def is_detour(straight_m: float, walk_m: float | None, ratio: float, excess: float) -> bool:
    return walk_m is not None and walk_m >= straight_m * ratio and walk_m - straight_m >= excess


def parse_tmap(body: dict) -> tuple[float, list[tuple[float, float]]]:
    """TMap 보행자 경로 응답(GeoJSON) → (총거리 m, [(lat, lon)…] 경로선)."""
    feats = body.get("features") or []
    if not feats:
        raise ValueError("features 없음")
    total = float(feats[0]["properties"]["totalDistance"])
    line: list[tuple[float, float]] = []
    for f in feats:
        g = f.get("geometry") or {}
        if g.get("type") == "LineString":
            line += [(c[1], c[0]) for c in g["coordinates"]]
    return total, line


def tmap_walk(key: str, o: dict, d: dict) -> tuple[float, list[tuple[float, float]]]:
    body = {"startX": o["lon"], "startY": o["lat"], "endX": d["lon"], "endY": d["lat"], "startName": "start",
            "endName": "end", "reqCoordType": "WGS84GEO", "resCoordType": "WGS84GEO"}
    req = urllib.request.Request(TMAP_URL, json.dumps(body).encode(),
                                 {"Content-Type": "application/json", "appKey": key})
    with urllib.request.urlopen(req, timeout=30) as r:
        return parse_tmap(json.load(r))


def write_md(path: str, rows: list[dict], flagged: list[dict], args: argparse.Namespace) -> None:
    ok = [r for r in rows if r["otp_m"] != ""]
    ratios = sorted(r["otp_m"] / r["straight_m"] for r in ok)

    def q(p: float) -> str:
        return f"{ratios[min(len(ratios) - 1, int(p * len(ratios)))]:.2f}" if ratios else "-"

    lines = [f"# 도보 우회 선별 ({datetime.date.today().isoformat()})", "",
             f"`otp/walk_detour_scan.py --per-origin {args.per_origin} --seed {args.seed}` 결과. "
             f"기준: OTP 도보 ÷ 직선 ≥ {args.ratio}, 초과 ≥ {args.excess:.0f}m.", "",
             "| 항목 | 값 |", "|---|---|",
             f"| 질의한 쌍 | {len(rows)} |", f"| OTP 경로 없음 | {len(rows) - len(ok)} |",
             f"| 비율 중앙값 / 90% / 99% | {q(0.5)} / {q(0.9)} / {q(0.99)} |",
             f"| 걸린 쌍 | {len(flagged)} |"]
    with_tmap = [f for f in flagged if f.get("tmap_m")]
    if with_tmap:
        worse = [f for f in with_tmap if f["otp_m"] >= f["tmap_m"] * 1.3]
        lines.append(f"| TMap 대조 / 우리가 1.3배 이상 김 | {len(with_tmap)} / {len(worse)} |")
    lines += ["", "## 걸린 쌍 (초과 거리 순, 최대 40건)", "",
              "| 출발 | 도착 | 직선(m) | OTP(m) | 비율 | TMap(m) |", "|---|---|---|---|---|---|"]
    for f in sorted(flagged, key=lambda f: f["straight_m"] - f["otp_m"])[:40]:
        lines.append(f"| {f['o_name']} ({f['o_lat']:.5f},{f['o_lon']:.5f}) | {f['d_name']} ({f['d_lat']:.5f},"
                     f"{f['d_lon']:.5f}) | {f['straight_m']:.0f} | {f['otp_m']:.0f} | "
                     f"{f['otp_m'] / f['straight_m']:.1f} | {f.get('tmap_m') or '-'} |")
    with open(path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines) + "\n")


def main() -> int:
    today = datetime.date.today().isoformat()
    ap = argparse.ArgumentParser()
    ap.add_argument("--otp", default="http://127.0.0.1:8083")
    ap.add_argument("--gbfs", default="http://127.0.0.1:8081/gbfs/station_information.json")
    ap.add_argument("--pbf", default=os.path.join(DATA, "seoul.osm.pbf"))
    ap.add_argument("--per-origin", type=int, default=4)
    ap.add_argument("--min-m", type=float, default=250)
    ap.add_argument("--max-m", type=float, default=1000)
    ap.add_argument("--ratio", type=float, default=2.0)
    ap.add_argument("--excess", type=float, default=400)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--workers", type=int, default=8)
    ap.add_argument("--tmap-limit", type=int, default=300)
    ap.add_argument("--out-md", default=os.path.normpath(os.path.join(DIR, "..", "docs", "eval",
                                                                      f"walk-detour-{today}.md")))
    a = ap.parse_args()
    rng = random.Random(a.seed)
    origins = load_origins(a.gbfs)
    dests = load_shops() + load_osm_pois(a.pbf)
    pairs = sample_pairs(origins, dests, a.per_origin, a.min_m, a.max_m, rng)
    print(f"출발 {len(origins)}, 도착 후보 {len(dests)}, 쌍 {len(pairs)}")

    def run(p):
        try:
            return otp_walk(a.otp, p[0], p[1])
        except OSError as e:
            print(f"OTP 오류: {e}", file=sys.stderr)
            return None

    t0 = time.time()
    with ThreadPoolExecutor(a.workers) as ex:
        walks = list(ex.map(run, pairs))
    print(f"OTP {len(pairs)}건 {time.time() - t0:.0f}초")

    rows, flagged = [], []
    for (o, d, s), w in zip(pairs, walks):
        row = {"o_name": o["name"], "o_lat": o["lat"], "o_lon": o["lon"], "d_name": d["name"], "d_kind": d["kind"],
               "d_lat": d["lat"], "d_lon": d["lon"], "straight_m": round(s, 1), "otp_m": "" if w is None else w}
        rows.append(row)
        if is_detour(s, w, a.ratio, a.excess):
            flagged.append(dict(row))
    key = tmap_key()
    if key:
        for f in sorted(flagged, key=lambda f: f["straight_m"] - f["otp_m"])[:a.tmap_limit]:
            o, d = {"lat": f["o_lat"], "lon": f["o_lon"]}, {"lat": f["d_lat"], "lon": f["d_lon"]}
            try:
                f["tmap_m"], f["tmap_line"] = tmap_walk(key, o, d)
            except (OSError, ValueError, KeyError) as e:
                print(f"TMap 실패: {e}", file=sys.stderr)
            time.sleep(0.2)
    with open(os.path.join(DATA, "walk-detour-pairs.csv"), "w", encoding="utf-8", newline="") as fh:
        wr = csv.DictWriter(fh, fieldnames=list(rows[0]))
        wr.writeheader()
        wr.writerows(rows)
    with open(os.path.join(DATA, "walk-detour-flagged.json"), "w", encoding="utf-8") as fh:
        json.dump(flagged, fh, ensure_ascii=False)
    write_md(a.out_md, rows, flagged, a)
    print(f"걸린 쌍 {len(flagged)} / {len(rows)} → {a.out_md}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
