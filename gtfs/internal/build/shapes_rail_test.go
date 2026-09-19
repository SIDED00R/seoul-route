package build

import (
	"math"
	"reflect"
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/geo"
	"github.com/SIDED00R/seoul-route/gtfs/internal/osm"
)

func TestRailRefs(t *testing.T) {
	cases := []struct {
		routeID, name string
		want          []string
	}{
		{"M_2", "2호선", []string{"2"}},
		{"M_9_X", "9호선", []string{"9"}},
		{"K_AP", "공항철도", []string{"공항철도", "AREX"}},
		{"K_WS", "우이신설경전철", []string{"W"}},
		{"RR_ACC1_S-1-SH-1D", "서해선", []string{"서해"}},
		{"RR_ACC1_S-1-ZZ-1D", "모르는 노선", nil},
	}
	for _, c := range cases {
		if got := railRefs(c.routeID, c.name); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v", c.routeID, got)
		}
	}
}

// 2호선 선로는 동쪽으로 불룩하게 휘어 있다(A 37.500 → 37.510,127.003 → C 37.520). 역 B 는 휜 꼭짓점에 있다.
func railFixture() ([]osm.RailWay, map[string]geo.Point) {
	ways := []osm.RailWay{{Ref: "2", ID: 1, Nodes: []osm.RailNode{
		{ID: 1, Lat: 37.500, Lon: 127.000}, {ID: 2, Lat: 37.510, Lon: 127.003}, {ID: 3, Lat: 37.520, Lon: 127.000}}}}
	stops := map[string]geo.Point{
		"A": {Lat: 37.5002, Lon: 127.0001}, "B": {Lat: 37.510, Lon: 127.0031}, "C": {Lat: 37.5198, Lon: 127.0001},
		"FAR": {Lat: 37.600, Lon: 127.000}, // 선로가 없는 곳(추출 범위 밖)
	}
	return ways, stops
}

func tripRow(route, id string) []string { return []string{route, "WEEKDAY", id, "", "0", ""} }

func stRows(trip string, stops ...string) [][]string {
	var out [][]string
	for i, s := range stops {
		out = append(out, []string{trip, "09:00:00", "09:00:00", s, itoa(i + 1), ""})
	}
	return out
}

func TestRailShapes(t *testing.T) {
	ways, stopAt := railFixture()
	trips := [][]string{tripRow("M_2", "local1"), tripRow("M_2", "local2"), tripRow("M_2_X", "express"),
		tripRow("M_2", "beyond"), tripRow("M_3", "noways"), tripRow("X_1", "unknown"),
		{"B_1", "ALL", "bus", "", "0", "BSH_1_0"}}
	var st [][]string
	st = append(st, stRows("local1", "A", "B", "C")...)
	st = append(st, stRows("local2", "A", "B", "C")...)
	st = append(st, stRows("express", "A", "C")...)
	st = append(st, stRows("beyond", "B", "C", "FAR")...)
	st = append(st, stRows("noways", "A", "B")...)
	st = append(st, stRows("unknown", "A", "B")...)
	st = append(st, []string{"bus", "00:00:00", "00:00:00", "BS_1", "1", "0.0"})
	names := map[string]string{"M_2": "2호선", "M_2_X": "2호선", "M_3": "3호선", "X_1": "모르는 노선"}
	shapes, stats := railShapes(ways, trips, st, names, stopAt)

	if trips[0][5] != "R_1" || trips[1][5] != "R_1" { // 정차 순서가 같으면 shape 하나를 같이 쓴다
		t.Errorf("완행 shape_id=%q,%q", trips[0][5], trips[1][5])
	}
	if trips[2][5] != "R_2" || trips[3][5] != "R_3" {
		t.Errorf("급행·범위 밖 shape_id=%q,%q", trips[2][5], trips[3][5])
	}
	if trips[4][5] != "" || trips[5][5] != "" { // 그 노선 선로가 없거나 모르는 노선
		t.Errorf("shape 가 없어야 하는 trip: %q,%q", trips[4][5], trips[5][5])
	}
	if trips[6][5] != "BSH_1_0" || st[len(st)-1][5] != "0.0" {
		t.Error("버스 행을 건드렸다")
	}
	if stats.Shapes != 3 || stats.NoShapeTrips != 2 || stats.StraightHops != 2 { // C>FAR(2호선), A>B(3호선)
		t.Errorf("stats=%+v", stats)
	}

	straight := geo.DistM(stopAt["A"], stopAt["C"])
	var r1, r2 [][]string
	for _, r := range shapes {
		switch r[0] {
		case "R_1":
			r1 = append(r1, r)
		case "R_2":
			r2 = append(r2, r)
		}
	}
	for _, rows := range [][][]string{r1, r2} { // 완행도 급행(B 통과)도 선로의 휜 꼭짓점을 지난다
		bulge := false
		for _, r := range rows {
			if parseF(r[2]) > 127.0029 {
				bulge = true
			}
		}
		length := parseF(rows[len(rows)-1][4])
		if !bulge || length < straight+20 { // 휜 선로 2,207m, 직선 2,179m
			t.Errorf("%s: 선로를 따라가지 않는다(길이 %.0f, 직선 %.0f)", rows[0][0], length, straight)
		}
	}
	// 완행의 정차별 거리: A 0, B 는 중간, C 는 shape 길이.
	a, b, c := parseF(st[0][5]), parseF(st[1][5]), parseF(st[2][5])
	if a != 0 || math.Abs(b-c/2) > 60 || math.Abs(c-parseF(r1[len(r1)-1][4])) > 0.11 {
		t.Errorf("정차별 거리 %v %v %v, shape 길이 %s", a, b, c, r1[len(r1)-1][4])
	}
	// shapeRows 가 구간 이음새의 같은 점을 한 번만 써서 거리는 점마다 늘어난다.
	for i := 1; i < len(shapes); i++ {
		if shapes[i][0] == shapes[i-1][0] && parseF(shapes[i][4]) <= parseF(shapes[i-1][4]) {
			t.Fatalf("거리가 늘지 않는 점: %v → %v", shapes[i-1], shapes[i])
		}
	}
	// 선로를 못 찾은 마지막 구간은 역 좌표를 직선으로 잇는다.
	var r3 [][]string
	for _, r := range shapes {
		if r[0] == "R_3" {
			r3 = append(r3, r)
		}
	}
	if last := r3[len(r3)-1]; last[1] != "37.600000" || last[2] != "127.000000" {
		t.Errorf("범위 밖 구간 끝점=%v", last)
	}
}

func TestPadRows(t *testing.T) {
	rows := [][]string{{"a", "b"}, {"a", "b", "c"}}
	padRows(rows, 3)
	if !reflect.DeepEqual(rows, [][]string{{"a", "b", ""}, {"a", "b", "c"}}) {
		t.Errorf("rows=%v", rows)
	}
}

// 역 좌표가 선로에서 100m 넘게 떨어져 있으면 shape 가 역 좌표를 거친다(OTP 의 150m 검사에 걸리지 않게).
func TestRailShapeVisitsFarStation(t *testing.T) {
	ways, stopAt := railFixture()
	stopAt["A"] = geo.Point{Lat: 37.5002, Lon: 126.9978} // 선로에서 서쪽으로 약 200m
	trips := [][]string{tripRow("M_2", "t")}
	st := stRows("t", "A", "B")
	shapes, _ := railShapes(ways, trips, st, map[string]string{"M_2": "2호선"}, stopAt)
	if len(shapes) < 3 || shapes[0][1] != "37.500200" || shapes[0][2] != "126.997800" {
		t.Fatalf("첫 점이 역 좌표가 아니다: %v", shapes[0])
	}
	if d := parseF(shapes[1][4]); d < 150 || d > 250 { // 역 → 선로에 붙은 자리
		t.Errorf("역에서 선로까지 %.0fm", d)
	}
	if last := shapes[len(shapes)-1]; parseF(last[2]) < 127.0029 { // B 는 선로 위라 그대로
		t.Errorf("끝점=%v", last)
	}
}
