"""patch_osm 검증. 실행: cd otp && python -m unittest test_patch_osm"""

import json
import os
import tempfile
import unittest

import osmium

import patch_osm
from patch_osm import Overrides


def write_pbf(path: str) -> None:
    """way 10(foot=no 산책로)·way 20(도로)과 노드 1~4. 노드 4 는 어느 way 에도 안 붙은 고립 노드."""
    with osmium.SimpleWriter(path) as w:
        for nid, (lat, lon) in {1: (37.5, 126.9), 2: (37.5, 126.901), 3: (37.501, 126.9), 4: (37.5, 126.8998)}.items():
            w.add(osmium.osm.mutable.Node(id=nid, location=(lon, lat), tags={}))
        w.add(osmium.osm.mutable.Way(id=10, nodes=[1, 2],
                                     tags={"highway": "service", "foot": "no", "name": "산책로 [공사중 폐쇄]"}))
        w.add(osmium.osm.mutable.Way(id=20, nodes=[2, 3], tags={"highway": "residential", "name": "길"}))
        w.add(osmium.osm.mutable.Relation(id=30, members=[("w", 10, ""), ("w", 20, "")], tags={"type": "route"}))


def read_ways(path: str) -> dict[int, tuple[list[int], dict[str, str]]]:
    return {w.id: ([n.ref for n in w.nodes], dict(w.tags)) for w in osmium.FileProcessor(path, osmium.osm.WAY)}


def count(path: str, kind) -> int:
    return sum(1 for _ in osmium.FileProcessor(path, kind))


class ApplyTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.src = os.path.join(self.dir, "src.osm.pbf")
        self.dst = os.path.join(self.dir, "dst.osm.pbf")
        write_pbf(self.src)

    def test_overrides_listed_way_only(self):
        applied, missing, _ = patch_osm.apply(self.src, self.dst,
                                              Overrides(tags={10: {"foot": "yes", "name": "산책로"}}))
        self.assertEqual((applied, missing), ({10}, set()))
        ways = read_ways(self.dst)
        self.assertEqual(ways[10], ([1, 2], {"highway": "service", "foot": "yes", "name": "산책로"}))
        self.assertEqual(ways[20], ([2, 3], {"highway": "residential", "name": "길"}))
        self.assertEqual(count(self.dst, osmium.osm.NODE), 4)
        self.assertEqual(count(self.dst, osmium.osm.RELATION), 1)

    def test_missing_way_is_reported_not_applied(self):
        applied, _, _ = patch_osm.apply(self.src, self.dst, Overrides(tags={99: {"foot": "yes"}}))
        self.assertEqual(applied, set())
        self.assertEqual(read_ways(self.dst)[10][1]["foot"], "no")

    def test_add_way_links_existing_nodes(self):
        ov = Overrides(add={900000000001: ([4, 1], {"highway": "footway"})})
        applied, missing, _ = patch_osm.apply(self.src, self.dst, ov)
        self.assertEqual((applied, missing), (set(), set()))
        ways = read_ways(self.dst)
        self.assertEqual(ways[900000000001], ([4, 1], {"highway": "footway"}))
        self.assertEqual(set(ways), {10, 20, 900000000001})
        # 새 way 는 기존 way 뒤·relation 앞에 놓인다(정렬된 pbf 유지)
        order = [(o.is_node(), o.is_way(), o.id) for o in osmium.FileProcessor(self.dst)]
        self.assertEqual([i for _, w, i in order if w], [10, 20, 900000000001])
        self.assertEqual(order[-1][2], 30)

    def test_add_way_without_relations_still_written(self):
        src = os.path.join(self.dir, "norel.osm.pbf")
        with osmium.SimpleWriter(src) as w:
            w.add(osmium.osm.mutable.Node(id=1, location=(126.9, 37.5), tags={}))
            w.add(osmium.osm.mutable.Node(id=2, location=(126.901, 37.5), tags={}))
        patch_osm.apply(src, self.dst, Overrides(add={900000000001: ([1, 2], {"highway": "footway"})}))
        self.assertEqual(list(read_ways(self.dst)), [900000000001])

    def test_add_way_reports_missing_nodes(self):
        ov = Overrides(add={900000000001: ([1, 77], {"highway": "footway"})})
        _, missing, _ = patch_osm.apply(self.src, self.dst, ov)
        self.assertEqual(missing, {77})

    def test_add_way_replaces_way_from_previous_patch(self):
        ov = Overrides(add={900000000001: ([4, 1], {"highway": "footway"})})
        patch_osm.apply(self.src, self.dst, ov)
        again = os.path.join(self.dir, "again.osm.pbf")
        ov2 = Overrides(add={900000000001: ([4, 2], {"highway": "path"})})
        patch_osm.apply(self.dst, again, ov2)
        ways = read_ways(again)
        self.assertEqual(ways[900000000001], ([4, 2], {"highway": "path"}))
        self.assertEqual(count(again, osmium.osm.WAY), 3)

    def test_idempotent(self):
        ov = Overrides(tags={10: {"foot": "yes"}})
        patch_osm.apply(self.src, self.dst, ov)
        again = os.path.join(self.dir, "again.osm.pbf")
        patch_osm.apply(self.dst, again, ov)
        self.assertEqual(read_ways(self.dst), read_ways(again))


class RuleTest(unittest.TestCase):
    """way 40 출입로 자전거길(foot 없음)·41 본선 자전거길·42 foot=no 나들목·43 출입로 footway."""

    RULE = patch_osm.Rule({"highway": "cycleway"}, patch_osm.re.compile("출입로|나들목"), ["foot"], {"foot": "yes"})

    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.src = os.path.join(self.dir, "src.osm.pbf")
        self.dst = os.path.join(self.dir, "dst.osm.pbf")
        with osmium.SimpleWriter(self.src) as w:
            for nid in range(1, 6):
                w.add(osmium.osm.mutable.Node(id=nid, location=(126.9 + nid / 1000, 37.5), tags={}))
            w.add(osmium.osm.mutable.Way(id=40, nodes=[1, 2], tags={"highway": "cycleway", "name": "중랑천 자전거길 출입로"}))
            w.add(osmium.osm.mutable.Way(id=41, nodes=[2, 3], tags={"highway": "cycleway", "name": "한강남자전거길"}))
            w.add(osmium.osm.mutable.Way(id=42, nodes=[3, 4],
                                         tags={"highway": "cycleway", "name": "잠원나들목", "foot": "no"}))
            w.add(osmium.osm.mutable.Way(id=43, nodes=[4, 5], tags={"highway": "footway", "name": "출입로"}))

    def test_rule_sets_only_matching_ways(self):
        _, _, hits = patch_osm.apply(self.src, self.dst, Overrides(rules=[self.RULE]))
        self.assertEqual(hits, [1])
        ways = read_ways(self.dst)
        self.assertEqual(ways[40][1].get("foot"), "yes")
        self.assertNotIn("foot", ways[41][1])  # 이름이 안 맞는다
        self.assertEqual(ways[42][1]["foot"], "no")  # 명시된 보행 금지는 둔다
        self.assertNotIn("foot", ways[43][1])  # highway 가 안 맞는다

    def test_rerun_counts_already_applied(self):
        patch_osm.apply(self.src, self.dst, Overrides(rules=[self.RULE]))
        again = os.path.join(self.dir, "again.osm.pbf")
        _, _, hits = patch_osm.apply(self.dst, again, Overrides(rules=[self.RULE]))
        self.assertEqual(hits, [1])
        self.assertEqual(read_ways(self.dst), read_ways(again))

    def test_way_override_wins_over_rule(self):
        patch_osm.apply(self.src, self.dst, Overrides(tags={40: {"foot": "designated"}}, rules=[self.RULE]))
        self.assertEqual(read_ways(self.dst)[40][1]["foot"], "designated")


class LoadOverridesTest(unittest.TestCase):
    def write(self, items) -> str:
        path = os.path.join(tempfile.mkdtemp(), "o.json")
        with open(path, "w", encoding="utf-8") as f:
            json.dump(items, f)
        return path

    def test_parses_all_kinds(self):
        path = self.write([{"way": 10, "tags": {"foot": "yes"}, "reason": "r", "checked": "2026-10-01"},
                           {"add_way": 900000000001, "nodes": [1, 2], "tags": {"highway": "footway"}},
                           {"rule": {"match": {"highway": "cycleway"}, "name_regex": "출입로", "unset": ["foot"]},
                            "tags": {"foot": "yes"}}])
        ov = patch_osm.load_overrides(path)
        self.assertEqual(ov.tags, {10: {"foot": "yes"}})
        self.assertEqual(ov.add, {900000000001: ([1, 2], {"highway": "footway"})})
        self.assertEqual(len(ov.rules), 1)
        self.assertTrue(ov.rules[0].matches({"highway": "cycleway", "name": "홍제천 자전거길 출입로"}))

    def test_rejects_bad_entries(self):
        bad = [
            [{"way": 10, "tags": {"a": "b"}}, {"way": 10, "tags": {"a": "c"}}],  # 중복
            [{"way": 10, "tags": {}}],  # 빈 태그
            [{"way": 10, "tags": {"a": 1}}],  # 문자열 아님
            [{"add_way": 1, "nodes": [5], "tags": {"highway": "footway"}}],  # 노드 1개
            [{"add_way": 1, "nodes": [5, 6], "tags": {}}],
            [{"tags": {"a": "b"}}],  # 종류 없음
            [{"rule": {"match": {}}, "tags": {"foot": "yes"}}],  # 빈 match 는 모든 way 에 걸린다
            [{"rule": {"match": {"highway": "cycleway"}}, "tags": {}}],
        ]
        for items in bad:
            with self.assertRaises(ValueError, msg=str(items)):
                patch_osm.load_overrides(self.write(items))

    def test_real_list_parses(self):
        ov = patch_osm.load_overrides(patch_osm.OVERRIDES)
        self.assertIn(1038687829, ov.tags)
        self.assertTrue(all(wid >= 900000000000 for wid in ov.add))


if __name__ == "__main__":
    unittest.main()
