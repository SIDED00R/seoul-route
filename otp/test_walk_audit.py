"""도보 OSM 결함 점검 도구(foot_graph·link_barriers·scan_closed_ways·walk_detour_scan·find_walk_gaps) 검증.

실행: cd otp && python -m unittest test_walk_audit
"""

import math
import os
import random
import tempfile
import unittest

import osmium

import find_walk_gaps as fw
import foot_graph as fg
import link_barriers as lb
import scan_closed_ways as sc
import walk_detour_scan as wd

LAT0, LON0 = 37.5, 126.9
DLAT, DLON = 1 / 111_000, 1 / 88_000  # 1m 를 도로 거칠게


def pt(x_m: float, y_m: float) -> tuple[float, float]:
    """원점에서 동쪽 x_m, 북쪽 y_m 인 (lat, lon)."""
    return LAT0 + y_m * DLAT, LON0 + x_m * DLON


def write_pbf(path: str, nodes: dict[int, tuple[float, float]], ways: list[tuple[int, list[int], dict]]) -> None:
    with osmium.SimpleWriter(path) as w:
        for nid, (lat, lon) in sorted(nodes.items()):
            w.add(osmium.osm.mutable.Node(id=nid, location=(lon, lat), tags={}))
        for wid, refs, tags in ways:
            w.add(osmium.osm.mutable.Way(id=wid, nodes=refs, tags=tags))


def pair(o: tuple[float, float], d: tuple[float, float], otp_m: float) -> dict:
    return {"o_lat": o[0], "o_lon": o[1], "d_lat": d[0], "d_lon": d[1], "o_name": "o", "d_name": "d",
            "straight_m": fg.haversine(*o, *d), "otp_m": otp_m}


FOOT = {"highway": "footway"}


class ClassifyTest(unittest.TestCase):
    def test_classes(self):
        cases = [
            ({"highway": "footway"}, fg.WALK),
            ({"highway": "footway", "foot": "no"}, fg.BLOCKED),
            ({"highway": "service", "access": "no"}, fg.BLOCKED),
            ({"highway": "service", "access": "no", "foot": "yes"}, fg.WALK),
            ({"highway": "construction"}, fg.BLOCKED),
            ({"highway": "primary", "foot": "no"}, fg.NONE),  # 큰길의 보행 금지는 후보가 아니다
            ({"highway": "primary", "foot": "use_sidepath"}, fg.NONE),
            ({"highway": "motorway"}, fg.NONE),
            ({"highway": "service", "access": "private"}, fg.PRIVATE_WALK),
            ({"highway": "service", "access": "private", "foot": "yes"}, fg.WALK),
            ({"highway": "cycleway"}, fg.CYCLE_ONLY),
            ({"highway": "cycleway", "foot": "designated"}, fg.WALK),
            ({"highway": "cycleway", "foot": "no"}, fg.BLOCKED),
            ({"building": "yes"}, fg.NONE),
        ]
        for tags, want in cases:
            self.assertEqual(fg.classify(tags), want, tags)


class GraphTest(unittest.TestCase):
    """o(1) ─ 막힌 footway ─ d(2) 직선 500m, 걷는 우회 1─3─4─2 는 북쪽 1km 로 돈다. 5─1 은 5 에서 끝나는 막다른 길."""

    @classmethod
    def setUpClass(cls):
        cls.pbf = os.path.join(tempfile.mkdtemp(), "g.osm.pbf")
        write_pbf(cls.pbf, {1: pt(0, 0), 2: pt(500, 0), 3: pt(0, 1000), 4: pt(500, 1000), 5: pt(-200, 0)}, [
            (10, [1, 2], {"highway": "footway", "foot": "no", "name": "산책로 [공사중 폐쇄]"}),
            (11, [1, 3, 4, 2], FOOT),
            (12, [5, 1], FOOT),
        ])
        cls.g = fg.load(cls.pbf)

    def test_blocked_way_is_not_in_graph(self):
        self.assertEqual(self.g.ways[10].cls, fg.BLOCKED)
        self.assertNotIn(2, [v for v, _, _ in self.g.adj[1]])
        self.assertAlmostEqual(self.g.ways[11].length_m, 2500, delta=30)

    def test_dead_end(self):
        self.assertTrue(self.g.is_dead_end(5))
        self.assertFalse(self.g.is_dead_end(1))  # 여러 way 가 만난다
        self.assertFalse(self.g.is_dead_end(3))  # way 중간

    def test_shortest_with_extra_reports_tag(self):
        bbox = (LON0 - 0.1, LAT0 - 0.1, LON0 + 0.1, LAT0 + 0.1)
        d0, used0 = fg.shortest(self.g, 1, 2, bbox)
        self.assertAlmostEqual(d0, 2500, delta=30)
        self.assertEqual(used0, [])
        extra = {1: [(2, 500.0, "X")], 2: [(1, 500.0, "X")]}
        d1, used1 = fg.shortest(self.g, 1, 2, bbox, extra)
        self.assertEqual((round(d1), used1), (500, ["X"]))

    def test_shortest_respects_bbox(self):
        tight = (LON0 - 0.001, LAT0 - 0.001, LON0 + 0.01, LAT0 + 0.002)  # 북쪽 우회가 밖이다
        self.assertEqual(fg.shortest(self.g, 1, 2, tight)[0], math.inf)

    def test_scan_closed_ways_lists_blocked_with_keyword(self):
        rows = sc.scan(self.g)
        self.assertEqual([r["way"] for r in rows], [10])
        self.assertEqual(rows[0]["reasons"], "foot=no;name~공사")


class BarrierTest(unittest.TestCase):
    def test_intersects(self):
        self.assertTrue(lb.intersects((0, 0), (1, 1), (0, 1), (1, 0)))
        self.assertFalse(lb.intersects((0, 0), (1, 1), (2, 0), (2, 1)))
        self.assertFalse(lb.intersects((0, 0), (1, 1), (1, 1), (2, 0)))  # 끝점이 닿기만 함

    def test_crossing_through_middle_vertex_of_barrier(self):
        idx = lb.BarrierIndex()
        idx.add(1, lb.HARD, [pt(10, -10), pt(10, 0), pt(10, 10)])  # 꼭짓점 3개 직선 담장
        self.assertEqual(idx.crossings(pt(0, 0), pt(20, 0)), {lb.HARD})  # 가운데 꼭짓점을 관통
        self.assertEqual(idx.crossings(pt(0, 0), pt(10, 0)), set())  # 연결 끝이 담장 중간 노드에 닿기만 함

    def test_kind(self):
        self.assertEqual(lb.kind({"barrier": "fence"}), lb.HARD)
        self.assertEqual(lb.kind({"railway": "rail"}), lb.HARD)
        self.assertEqual(lb.kind({"railway": "rail", "bridge": "yes"}), "")  # 고가 밑은 지나간다
        self.assertEqual(lb.kind({"highway": "secondary"}), lb.ROAD)
        self.assertEqual(lb.kind({"highway": "footway"}), "")


class GapTest(unittest.TestCase):
    """구역마다 원점에서 동쪽으로 수 km 떨어져 서로 영향이 없다.

    A(x 0)    : 직선 500m 를 막힌 footway 가 잇고, 걷는 길은 북쪽 3km 우회 → R
    B(x 5000) : 출발 footway 가 남쪽 끝(101)에서 15m 모자라 도착 footway(102)에 못 닿는다 → L, 등급 A
    C(x 10000): B 와 같은데 도착 쪽 길이 진출입 램프 → 잇지 않는다
    D(x 15000): private 도로로 바로 가는데 OTP 는 3km 를 돌았다고 한다 → model
    E(x 20000): B 와 같은데 틈 사이를 담장이 막는다 → 잇지 않는다
    F(x 25000): 직선을 보행 표시 없는 자전거길이 잇는다 → R(자전거길)
    G(x 30000): B 와 같은데 틈 45m 사이를 2차로 도로가 지난다 → L, 등급 B·큰길 가로지름
    H(x 35000): B 와 같은데 이을 노드가 다리 중간이다 → 잇지 않는다
    I(x 40000): o─m 은 막힌 길(250m)과 걷는 곁길(약 297m), m─d 는 막힌 길(250m)뿐이고 걷는 길은 3km 우회
                → 둘 다 열면 500m 지만 o─m 을 열어도 47m 밖에 안 줄어 최소 묶음은 m─d 하나
    """

    @classmethod
    def setUpClass(cls):
        pbf = os.path.join(tempfile.mkdtemp(), "gap.osm.pbf")
        n = {1: pt(0, 0), 2: pt(500, 0), 3: pt(0, 3000), 4: pt(500, 3000),
             300: pt(15000, 0), 301: pt(15500, 0)}
        ways = [(10, [1, 2], {"highway": "footway", "foot": "no"}), (11, [1, 3, 4, 2], FOOT),
                (40, [300, 301], {"highway": "service", "access": "private"})]
        # B·C·E·G·H: o(x) ─ 끝(x+200) … 틈 … (x+200+gap) ─ d(x+500)
        for base, gap, dest_tags in [(5000, 15, FOOT), (10000, 15, {"highway": "primary_link"}),
                                     (20000, 15, FOOT), (30000, 45, FOOT), (35000, 15, None)]:
            o, e, s, d = base * 10, base * 10 + 1, base * 10 + 2, base * 10 + 3
            n |= {o: pt(base, 0), e: pt(base + 200, 0), s: pt(base + 200 + gap, 0), d: pt(base + 500, 0)}
            ways.append((o, [o, e], FOOT))
            if dest_tags is None:  # H: s 가 다리 way 의 중간 노드
                n[d + 1] = pt(base + 200 + gap, -100)
                ways.append((d, [d + 1, s, d], {"highway": "footway", "bridge": "yes", "layer": "1"}))
            else:
                ways.append((d, [s, d], dest_tags))
        n |= {290001: pt(20207, -50), 290002: pt(20207, 50)}  # E 의 담장
        ways.append((290001, [290001, 290002], {"barrier": "fence"}))
        n |= {390001: pt(30220, -200), 390002: pt(30220, 200)}  # G 의 도로
        ways.append((390001, [390001, 390002], {"highway": "secondary"}))
        n |= {250000: pt(25000, 0), 250001: pt(25500, 0), 250002: pt(25000, 3000), 250003: pt(25500, 3000)}
        ways += [(250000, [250000, 250001], {"highway": "cycleway"}),
                 (250001, [250000, 250002, 250003, 250001], FOOT)]
        n |= {400000: pt(40000, 0), 400001: pt(40250, 0), 400002: pt(40500, 0), 400003: pt(40125, 80),
              400004: pt(40250, 3000), 400005: pt(40500, 3000)}
        ways += [(400000, [400000, 400001], {"highway": "footway", "foot": "no"}),
                 (400001, [400000, 400003, 400001], FOOT),
                 (400002, [400001, 400002], {"highway": "footway", "foot": "no"}),
                 (400003, [400001, 400004, 400005, 400002], FOOT)]
        write_pbf(pbf, n, ways)
        cls.n = n
        cls.ctx = fw.make_ctx(fg.load(pbf), lb.load(pbf))

    def explain(self, o, d, otp_m):
        return fw.explain(self.ctx, pair(self.n[o], self.n[d], otp_m))

    def test_reopen_blocked_way(self):
        r = self.explain(1, 2, 7000)
        self.assertEqual(r["category"], "fixable")
        self.assertEqual(r["set"], [("R", 10)])
        self.assertAlmostEqual(r["fixed_m"], 500, delta=10)
        self.assertFalse(fw.describe(self.ctx, ("R", 10))["cycle_only"])

    def test_link_dead_end_grade_a(self):
        r = self.explain(50000, 50003, 4000)
        self.assertEqual(r["category"], "fixable")
        self.assertEqual(r["set"], [("L", 50001, 50002)])
        self.assertAlmostEqual(r["fixed_m"], 500, delta=10)
        c = fw.describe(self.ctx, r["set"][0])
        self.assertEqual((c["length_m"], c["grade"], c["crosses_road"], c["override"]["nodes"]),
                         (15, "A", False, [50001, 50002]))

    def test_no_link_to_ramp(self):
        self.assertEqual(self.explain(100000, 100003, 4000)["category"], "unexplained")

    def test_private_shortcut_is_model(self):
        r = self.explain(300, 301, 3000)
        self.assertEqual(r["category"], "model")
        self.assertEqual(r["private"], [40])

    def test_no_link_through_fence(self):
        self.assertEqual(self.explain(200000, 200003, 4000)["category"], "unexplained")

    def test_cycleway_without_foot_is_reopen_candidate(self):
        r = self.explain(250000, 250001, 7000)
        self.assertEqual(r["set"], [("R", 250000)])
        self.assertTrue(fw.describe(self.ctx, ("R", 250000))["cycle_only"])

    def test_link_across_road_is_grade_b(self):
        r = self.explain(300000, 300003, 4000)
        self.assertEqual(r["set"], [("L", 300001, 300002)])
        c = fw.describe(self.ctx, r["set"][0])
        self.assertEqual((c["grade"], c["crosses_road"]), ("B", True))

    def test_no_link_into_bridge_middle(self):
        self.assertEqual(self.explain(350000, 350003, 4000)["category"], "unexplained")

    def test_minimal_set_drops_candidate_with_small_gain(self):
        r = self.explain(400000, 400002, 6000)
        self.assertEqual(r["set"], [("R", 400002)])
        self.assertAlmostEqual(r["fixed_m"], 547, delta=10)

    def test_no_detour_left_is_unexplained(self):
        self.assertEqual(self.explain(1, 2, 520)["category"], "unexplained")

    def test_aggregate_counts_and_tmap(self):
        f1 = pair(self.n[1], self.n[2], 7000)
        f2 = pair(self.n[1], self.n[2], 6000) | {"tmap_line": [self.n[1], self.n[2]]}
        f3 = pair(self.n[1], self.n[2], 6500) | {"tmap_line": [self.n[3], self.n[4]]}
        rs = [fw.explain(self.ctx, f) for f in (f1, f2, f3)]
        cands = fw.aggregate(self.ctx, [f1, f2, f3], rs)
        self.assertEqual(len(cands), 1)
        c = cands[0]
        self.assertEqual((c["type"], c["way"], c["pairs"], c["solo"]), ("R", 10, 3, 3))
        self.assertEqual(c["best_saving_m"], round(7000 - rs[0]["fixed_m"]))
        # 막힌 way(노드 1→2)는 f2 의 TMap 선과 겹치고, f3 의 선(북쪽 3km)에서는 멀다
        self.assertEqual((c["tmap_on"], c["tmap_off"]), (1, 1))

    def test_point_line_measures_to_segment_not_vertex(self):
        line = [pt(0, 0), pt(500, 0)]  # 꼭짓점 2개, 500m
        self.assertAlmostEqual(fw.point_line_m(*pt(250, 10), line), 10, delta=1)  # 가운데 위 10m
        self.assertAlmostEqual(fw.point_line_m(*pt(600, 0), line), 100, delta=2)  # 끝 너머는 끝점까지
        self.assertEqual(fw.point_line_m(*pt(0, 0), []), math.inf)

    def test_on_tmap_link_needs_both_ends_and_way_needs_overlap(self):
        line = [pt(0, -10), pt(0, 3000)]  # 서쪽 끝(x=0)을 따라 북쪽으로
        self.assertFalse(fw.on_tmap(self.ctx, ("R", 10), line))  # 막힌 way 1→2 는 x 0~500, 선과 겹치는 건 끝 25m 뿐
        self.assertTrue(fw.on_tmap(self.ctx, ("R", 10), [pt(-10, 3), pt(510, 3)]))  # 나란히 3m 옆
        self.assertTrue(fw.on_tmap(self.ctx, ("L", 50001, 50002), [pt(5100, 5), pt(5300, 5)]))
        # 남북으로 지나는 선이 끝 50001(x 5200)에서 15m, 50002(x 5215)에서 30m — 한쪽 끝만 가깝다
        self.assertFalse(fw.on_tmap(self.ctx, ("L", 50001, 50002), [pt(5185, -100), pt(5185, 100)]))


class DetourScanTest(unittest.TestCase):
    def test_is_detour(self):
        self.assertTrue(wd.is_detour(500, 1500, 2.0, 400))
        self.assertFalse(wd.is_detour(500, 900, 2.0, 400))  # 비율 미달
        self.assertFalse(wd.is_detour(250, 600, 2.0, 400))  # 초과 350m 미달
        self.assertFalse(wd.is_detour(500, None, 2.0, 400))

    def test_sample_pairs_within_band(self):
        origins = [{"lat": LAT0, "lon": LON0, "name": "o"}]
        dests = [{"lat": pt(x, 0)[0], "lon": pt(x, 0)[1], "name": str(x), "kind": "shop"} for x in range(0, 2000, 50)]
        pairs = wd.sample_pairs(origins, dests, 5, 250, 1000, random.Random(1))
        self.assertEqual(len(pairs), 5)
        for _, _, s in pairs:
            self.assertTrue(250 <= s <= 1000, s)
        self.assertEqual(len(wd.sample_pairs(origins, dests, 99, 250, 1000, random.Random(1))), 15)

    def test_parse_tmap(self):
        body = {"type": "FeatureCollection", "features": [
            {"type": "Feature", "geometry": {"type": "Point", "coordinates": [126.9, 37.5]},
             "properties": {"totalDistance": 812, "totalTime": 600}},
            {"type": "Feature", "geometry": {"type": "LineString", "coordinates": [[126.9, 37.5], [126.901, 37.5]]},
             "properties": {"distance": 88}},
            {"type": "Feature", "geometry": {"type": "LineString", "coordinates": [[126.901, 37.5], [126.902, 37.501]]},
             "properties": {"distance": 120}},
        ]}
        total, line = wd.parse_tmap(body)
        self.assertEqual(total, 812.0)
        self.assertEqual(line[0], (37.5, 126.9))
        self.assertEqual(len(line), 4)
        with self.assertRaises(ValueError):
            wd.parse_tmap({"features": []})

    def test_dotenv_value(self):
        path = os.path.join(tempfile.mkdtemp(), ".env")
        with open(path, "w", encoding="utf-8") as f:
            f.write("# 주석\nOTHER=1\n TMAP_APP_KEY = 'ab$c=d' \nEMPTY=\n")
        self.assertEqual(wd.dotenv_value(path, "TMAP_APP_KEY"), "ab$c=d")  # 첫 '=' 에서 나누고 따옴표를 벗긴다
        self.assertEqual(wd.dotenv_value(path, "NONE"), "")
        self.assertEqual(wd.dotenv_value(path + ".missing", "TMAP_APP_KEY"), "")

    def test_poi_kind(self):
        self.assertEqual(wd.poi_kind({"leisure": "pitch"}), "leisure=pitch")
        self.assertEqual(wd.poi_kind({"amenity": "bench"}), "")


if __name__ == "__main__":
    unittest.main()
