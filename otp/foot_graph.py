"""OSM pbf 에서 보행 그래프를 만든다. scan_closed_ways·find_walk_gaps 가 같이 쓴다.

way 를 다섯 갈래로 나눈다(classify):
  walk     걷는다
  private  걷는다고 보되 따로 표시한다(access=private 등 — OTP 는 통과 교통을 막아 돌아갈 수 있다)
  blocked  지금은 못 걷지만 다시 열 후보다(길 성격 way 의 foot=no·access=no, highway=construction)
  cycle    보행 표시(foot)가 없는 자전거길. OTP 기본 매핑이 걷지 못하게 한다(2026-10-01 실측: foot 없는 cycleway 12곳
           모두 OTP 도보가 길 길이보다 길고, foot=yes 12곳은 같다). 다시 열 후보로 다룬다
  none     못 걷고 후보도 아니다(자동차 전용, 큰길의 foot=no·use_sidepath 등)
OTP 'default' 태그 매핑을 거칠게 흉내 낸 것이다. 면(area=yes)은 둘레만 잇는다(OTP 는 면 안을 가로지른다).
"""

import heapq
import math
from dataclasses import dataclass, field

import osmium

NONE_HIGHWAY = {"motorway", "motorway_link", "proposed", "abandoned", "razed", "raceway", "bus_guideway", "busway",
                "escape", "corridor_proposed"}
# 다시 열 후보가 될 수 있는 길 성격 highway. 큰길(trunk·primary 등)의 foot=no 는 실제 보행 금지일 때가 많아 뺀다.
PATHLIKE_HIGHWAY = {"footway", "path", "pedestrian", "steps", "cycleway", "service", "residential", "living_street",
                    "track", "unclassified", "bridleway", "corridor", "construction"}
FOOT_ALLOW = {"yes", "designated", "permissive"}
DENY = {"no"}
PRIVATE = {"private", "customers", "destination", "delivery"}

WALK, PRIVATE_WALK, BLOCKED, CYCLE_ONLY, NONE = "walk", "private", "blocked", "cycle", "none"
CANDIDATE = {BLOCKED, CYCLE_ONLY}  # 그래프에 넣지 않고 다시 열 후보로 두는 갈래


def classify(tags: dict[str, str]) -> str:
    h = tags.get("highway")
    if not h or h in NONE_HIGHWAY:
        return NONE
    foot = tags.get("foot", "")
    access = tags.get("access", "")
    pathlike = h in PATHLIKE_HIGHWAY
    if h == "construction":
        return BLOCKED
    if foot == "use_sidepath":
        return NONE
    if foot in DENY or (access in DENY and foot not in FOOT_ALLOW):
        return BLOCKED if pathlike else NONE
    if h == "cycleway" and foot not in FOOT_ALLOW:
        return CYCLE_ONLY
    if foot in PRIVATE or (access in PRIVATE and foot not in FOOT_ALLOW):
        return PRIVATE_WALK
    return WALK


def haversine(lat1: float, lon1: float, lat2: float, lon2: float) -> float:
    r = 6371008.8
    p1, p2 = math.radians(lat1), math.radians(lat2)
    a = math.sin((p2 - p1) / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(math.radians(lon2 - lon1) / 2) ** 2
    return 2 * r * math.asin(math.sqrt(a))


@dataclass
class Way:
    id: int
    cls: str
    nodes: list[int]
    tags: dict[str, str]
    timestamp: str  # ISO 날짜(편집일). pbf 에 없으면 빈 값
    length_m: float


CELL = 0.001  # 격자 칸(도). 위도 약 111m


@dataclass
class FootGraph:
    coords: dict[int, tuple[float, float]] = field(default_factory=dict)
    adj: dict[int, list[tuple[int, float, int]]] = field(default_factory=dict)  # 노드 → (이웃, 길이, way id)
    ways: dict[int, Way] = field(default_factory=dict)  # walk·private·blocked way
    node_ways: dict[int, list[int]] = field(default_factory=dict)  # 노드 → 그 노드를 쓰는 way(walk·private·blocked)
    grid: dict[tuple[int, int], list[int]] = field(default_factory=dict)  # 걷는 그래프 노드의 격자 색인

    def add_way(self, way: Way, locs: list[tuple[float, float]]) -> None:
        self.ways[way.id] = way
        for nid, loc in zip(way.nodes, locs):
            self.coords[nid] = loc
            self.node_ways.setdefault(nid, []).append(way.id)
        if way.cls in CANDIDATE:
            return
        for a, b in zip(way.nodes, way.nodes[1:]):
            if a == b:
                continue
            d = haversine(*self.coords[a], *self.coords[b])
            self.adj.setdefault(a, []).append((b, d, way.id))
            self.adj.setdefault(b, []).append((a, d, way.id))

    def build_grid(self) -> None:
        self.grid.clear()
        for nid in self.adj:
            lat, lon = self.coords[nid]
            self.grid.setdefault((int(lat / CELL), int(lon / CELL)), []).append(nid)

    def near(self, lat: float, lon: float, radius_m: float) -> list[tuple[float, int]]:
        """(lat, lon) 에서 radius_m 안의 걷는 그래프 노드를 가까운 순으로."""
        k = int(radius_m / 80) + 1  # 경도 칸은 서울에서 약 88m
        ci, cj = int(lat / CELL), int(lon / CELL)
        out = []
        for i in range(ci - k, ci + k + 1):
            for j in range(cj - k, cj + k + 1):
                for nid in self.grid.get((i, j), ()):
                    d = haversine(lat, lon, *self.coords[nid])
                    if d <= radius_m:
                        out.append((d, nid))
        out.sort()
        return out

    def is_dead_end(self, nid: int) -> bool:
        """걷는 way 하나의 끝점이고 다른 way(blocked·cycle 포함)와 만나지 않는 노드."""
        ws = self.node_ways.get(nid, [])
        if len(ws) != 1:
            return False
        w = self.ways[ws[0]]
        if w.nodes[0] == w.nodes[-1]:
            return False
        return nid in (w.nodes[0], w.nodes[-1])


def load(pbf: str, bbox: tuple[float, float, float, float] | None = None) -> FootGraph:
    """pbf 의 highway way 로 보행 그래프를 만든다. bbox=(min_lon, min_lat, max_lon, max_lat) 를 주면 그 안에 노드가
    하나라도 있는 way 만 넣는다."""
    g = FootGraph()
    for o in osmium.FileProcessor(pbf).with_locations():
        if not o.is_way() or "highway" not in o.tags:
            continue
        tags = dict(o.tags)
        cls = classify(tags)
        if cls == NONE:
            continue
        nodes, locs = [], []
        for n in o.nodes:
            if not n.location.valid():
                continue
            nodes.append(n.ref)
            locs.append((n.location.lat, n.location.lon))
        if len(nodes) < 2:
            continue
        if bbox and not any(bbox[0] <= lo <= bbox[2] and bbox[1] <= la <= bbox[3] for la, lo in locs):
            continue
        length = sum(haversine(*a, *b) for a, b in zip(locs, locs[1:]))
        ts = o.timestamp.date().isoformat() if o.timestamp and o.timestamp.year > 1970 else ""
        g.add_way(Way(o.id, cls, nodes, tags, ts, length), locs)
    g.build_grid()
    return g


Extra = dict[int, list[tuple[int, float, object]]]  # 노드 → (이웃, 길이, 표지) — 후보 간선


def in_bbox(c: tuple[float, float], bbox: tuple[float, float, float, float]) -> bool:
    return bbox[0] <= c[1] <= bbox[2] and bbox[1] <= c[0] <= bbox[3]


def shortest(g: FootGraph, src: int, dst: int, bbox: tuple[float, float, float, float],
             extra: Extra | None = None) -> tuple[float, list[object]]:
    """bbox 안에서 src→dst 최단 거리(A*)와 그 경로가 지난 표지 목록(순서대로, 중복 없음). 없으면 (inf, []).

    표지는 extra 간선의 표지와, private way 를 지난 경우 ("P", way id) 다.
    """
    extra = extra or {}
    tlat, tlon = g.coords[dst]
    dist = {src: 0.0}
    prev: dict[int, tuple[int, object]] = {}
    pq = [(haversine(*g.coords[src], tlat, tlon), src)]
    done = set()
    while pq:
        _, u = heapq.heappop(pq)
        if u == dst:
            break
        if u in done:
            continue
        done.add(u)
        du = dist[u]
        steps = [(v, w, ("P", wid) if g.ways[wid].cls == PRIVATE_WALK else None) for v, w, wid in g.adj.get(u, ())]
        for v, w, tag in steps + extra.get(u, []):
            cv = g.coords[v]
            if v in done or not in_bbox(cv, bbox):
                continue
            nd = du + w
            if nd < dist.get(v, math.inf):
                dist[v] = nd
                prev[v] = (u, tag)
                heapq.heappush(pq, (nd + haversine(cv[0], cv[1], tlat, tlon), v))
    if dst not in dist:
        return math.inf, []
    used, n = [], dst
    while n != src:
        n, tag = prev[n]
        if tag is not None and tag not in used:
            used.append(tag)
    used.reverse()
    return dist[dst], used
