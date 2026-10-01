"""잇기 후보 선분이 담장·철로·물·고속도로 같은 장벽을 가로지르는지 본다(find_walk_gaps 의 L 후보 거르기, 이슈 #176).

HARD 를 가로지르면 잇지 않는다. ROAD(일반 큰길)를 가로지르면 횡단보도 없이 건너는 연결이라 사람이 확인할 몫으로 남긴다.
다리(bridge)·터널(tunnel) 위·아래의 장벽은 땅 위 연결을 막지 않으므로 뺀다.
"""

import osmium

HARD, ROAD = "hard", "road"
BARRIER = {"fence", "wall", "retaining_wall", "hedge", "guard_rail", "city_wall", "jersey_barrier", "handrail"}
RAILWAY = {"rail", "subway", "light_rail", "narrow_gauge", "monorail", "tram", "preserved", "funicular"}
WATERWAY = {"river", "stream", "canal", "ditch", "drain", "riverbank"}
HARD_HIGHWAY = {"motorway", "motorway_link", "trunk", "trunk_link"}
ROAD_HIGHWAY = {"primary", "primary_link", "secondary", "secondary_link", "tertiary", "tertiary_link"}
CELL = 0.002


def kind(tags) -> str:
    if tags.get("bridge", "no") != "no" or tags.get("tunnel", "no") != "no" or tags.get("covered") == "yes":
        return ""
    if tags.get("barrier") in BARRIER or tags.get("railway") in RAILWAY or tags.get("waterway") in WATERWAY:
        return HARD
    if tags.get("natural") == "water":
        return HARD
    h = tags.get("highway")
    if h in HARD_HIGHWAY:
        return HARD
    if h in ROAD_HIGHWAY:
        return ROAD
    return ""


Seg = tuple[float, float, float, float, str, int]  # lat1, lon1, lat2, lon2, 종류, way id


class BarrierIndex:
    def __init__(self) -> None:
        self.grid: dict[tuple[int, int], list[Seg]] = {}

    def add(self, way_id: int, k: str, locs: list[tuple[float, float]]) -> None:
        for (a, b), (c, d) in zip(locs, locs[1:]):
            seg = (a, b, c, d, k, way_id)
            for i in range(int(min(a, c) / CELL), int(max(a, c) / CELL) + 1):
                for j in range(int(min(b, d) / CELL), int(max(b, d) / CELL) + 1):
                    self.grid.setdefault((i, j), []).append(seg)

    def crossings(self, p: tuple[float, float], q: tuple[float, float], skip_ways: set[int] = frozenset()) -> set[str]:
        """선분 p→q 가 가로지르는 장벽 종류. skip_ways(이을 양끝의 way)는 보지 않는다."""
        out: set[str] = set()
        seen: set[int] = set()
        for i in range(int(min(p[0], q[0]) / CELL), int(max(p[0], q[0]) / CELL) + 1):
            for j in range(int(min(p[1], q[1]) / CELL), int(max(p[1], q[1]) / CELL) + 1):
                for s in self.grid.get((i, j), ()):
                    if s[5] in skip_ways or id(s) in seen:
                        continue
                    seen.add(id(s))
                    if intersects(p, q, (s[0], s[1]), (s[2], s[3])):
                        out.add(s[4])
        return out


def _orient(a, b, c) -> float:
    return (b[0] - a[0]) * (c[1] - a[1]) - (b[1] - a[1]) * (c[0] - a[0])


def intersects(p, q, r, s) -> bool:
    """연결 pq 가 장벽 선분 rs 를 가로지르는지. pq 끝점이 rs 에 닿기만 하는 것은 교차가 아니고, rs 끝점 하나만(장벽
    way 의 중간 꼭짓점일 수 있다) pq 안쪽에 놓이면 교차다. rs 가 pq 와 한 직선 위에 겹치면 교차가 아니다."""
    d1, d2 = _orient(r, s, p), _orient(r, s, q)
    d3, d4 = _orient(p, q, r), _orient(p, q, s)
    eps = 1e-14
    on_r, on_s = abs(d3) <= eps, abs(d4) <= eps
    return (d1 > eps and d2 < -eps or d1 < -eps and d2 > eps) and (
        d3 > eps and d4 < -eps or d3 < -eps and d4 > eps or on_r != on_s)


def load(pbf: str) -> BarrierIndex:
    idx = BarrierIndex()
    for o in osmium.FileProcessor(pbf).with_locations():
        if not o.is_way():
            continue
        k = kind(o.tags)
        if not k:
            continue
        locs = [(n.location.lat, n.location.lon) for n in o.nodes if n.location.valid()]
        if len(locs) >= 2:
            idx.add(o.id, k, locs)
    return idx
