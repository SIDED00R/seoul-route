package build

import (
	"archive/zip"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/geo"
	"github.com/SIDED00R/seoul-route/gtfs/internal/seoulbus"
)

// 왕복 노선: 경도 127.0000 을 따라 북쪽으로 1.1km 올라갔다가 동쪽으로 18m 옆(127.0002) 길로 되돌아온다.
func loopPath() []seoulbus.PathPoint {
	var out []seoulbus.PathPoint
	add := func(lat, lon float64) {
		out = append(out, seoulbus.PathPoint{No: fmt.Sprint(len(out) + 1), Lat: fmt.Sprintf("%.6f", lat),
			Lon: fmt.Sprintf("%.6f", lon)})
	}
	for i := 0; i <= 10; i++ {
		add(37.500+float64(i)*0.001, 127.0000)
	}
	for i := 10; i >= 0; i-- {
		add(37.500+float64(i)*0.001, 127.0002)
	}
	return out
}

func busStop(seq int, lat, lon float64, sect int, turn bool) seoulbus.Stop {
	yn := "N"
	if turn {
		yn = "Y"
	}
	return seoulbus.Stop{Seq: fmt.Sprint(seq), StationID: fmt.Sprint(1000 + seq), Name: fmt.Sprint("S", seq),
		Lat: fmt.Sprintf("%.6f", lat), Lon: fmt.Sprintf("%.6f", lon), SectDist: fmt.Sprint(sect), SectSpd: "20", TransYn: yn}
}

// 올라가는 길의 정류장 S2 는 되돌아오는 길에 더 가깝게 찍혀 있다(4m 대 13m).
func loopStops() []seoulbus.Stop {
	return []seoulbus.Stop{
		busStop(1, 37.501, 126.9999, 0, false),
		busStop(2, 37.505, 127.00015, 445, false),
		busStop(3, 37.5095, 126.9999, 500, true),
		busStop(4, 37.505, 127.0003, 520, false),
		busStop(5, 37.501, 127.0003, 445, false),
	}
}

func matched(t *testing.T, stops []seoulbus.Stop) ([]geo.Point, []float64, []busStopMatch) {
	t.Helper()
	pts := busPathPoints(loopPath())
	cum := geo.Cumulative(pts)
	return pts, cum, matchBusStops(pts, cum, stops)
}

func TestMatchBusStopsKeepsOrderOnLoop(t *testing.T) {
	_, _, m := matched(t, loopStops())
	want := []float64{111, 556, 1056, 1686, 2131} // 올라가는 길 1,112m + 옆 길로 건너가는 18m 뒤에 되돌아오는 길이 시작한다
	for i, w := range want {
		if math.Abs(m[i].alongM-w) > 8 {
			t.Errorf("S%d alongM=%.0f want≈%.0f", i+1, m[i].alongM, w)
		}
		if m[i].distM > 20 {
			t.Errorf("S%d distM=%.0f", i+1, m[i].distM)
		}
	}
}

func TestBusShapePerDirection(t *testing.T) {
	stops := loopStops()
	pts, cum, m := matched(t, stops)
	up, dUp, ok := busShape(pts, cum, m, stops, []int{0, 1, 2})
	if !ok {
		t.Fatal("상행 shape 없음")
	}
	for _, p := range up {
		if p.Lon != 127.0 {
			t.Fatalf("상행 shape 가 되돌아오는 길을 포함한다: %v", p)
		}
	}
	if dUp[0] != 0 || math.Abs(dUp[1]-445) > 8 || math.Abs(dUp[2]-945) > 8 {
		t.Errorf("상행 거리=%v", dUp)
	}
	upCum := geo.Cumulative(up)
	if math.Abs(upCum[len(upCum)-1]-dUp[2]) > 0.5 { // 마지막 정류장의 거리 = shape 길이
		t.Errorf("shape 길이 %.1f, 마지막 정류장 %.1f", upCum[len(upCum)-1], dUp[2])
	}
	down, dDown, ok := busShape(pts, cum, m, stops, []int{2, 3, 4})
	if !ok {
		t.Fatal("하행 shape 없음")
	}
	if first, last := down[0], down[len(down)-1]; first.Lon != 127.0 || last.Lon != 127.0002 || last.Lat > 37.5011 {
		t.Errorf("하행 shape 양 끝: %v → %v", first, last)
	}
	if !(dDown[0] == 0 && dDown[1] > 600 && dDown[2] > dDown[1]) {
		t.Errorf("하행 거리=%v", dDown)
	}
}

// 차고지 기점처럼 끝 정류장만 경로 밖이면 그 정류장 좌표를 shape 끝에 붙인다.
func TestBusShapeTerminalOffPath(t *testing.T) {
	stops := loopStops()
	stops[0] = busStop(1, 37.501, 126.9972, 0, false) // 서쪽 250m
	pts, cum, m := matched(t, stops)
	shape, d, ok := busShape(pts, cum, m, stops, []int{0, 1, 2})
	if !ok {
		t.Fatal("shape 없음")
	}
	if shape[0].Lon != 126.9972 || shape[1].Lon != 127.0 {
		t.Errorf("앞머리: %v, %v", shape[0], shape[1])
	}
	if d[0] != 0 || math.Abs(d[1]-(m[0].distM+445)) > 8 {
		t.Errorf("거리=%v (정류장→경로 %.0fm)", d, m[0].distM)
	}
}

func TestBusShapeRejects(t *testing.T) {
	base := loopStops()
	five := []int{0, 1, 2, 3, 4}
	cases := []struct {
		name string
		edit func(s []seoulbus.Stop)
		ok   bool
	}{
		{"그대로", func(s []seoulbus.Stop) {}, true},
		{"가운데 하나가 150m 어긋남", func(s []seoulbus.Stop) { s[1] = busStop(2, 37.505, 126.9983, 445, false) }, true},
		{"가운데 둘이 150m 어긋남", func(s []seoulbus.Stop) {
			s[1] = busStop(2, 37.505, 126.9983, 445, false)
			s[3] = busStop(4, 37.505, 127.0019, 520, false)
		}, false},
		{"가운데 하나가 400m 어긋남", func(s []seoulbus.Stop) { s[1] = busStop(2, 37.505, 126.9955, 445, false) }, false},
		{"같은 자리에 두 번 선다", func(s []seoulbus.Stop) { s[1] = s[0] }, false},
		{"기점이 400m 어긋남", func(s []seoulbus.Stop) { s[0] = busStop(1, 37.501, 126.9955, 0, false) }, false},
		{"종점이 400m 어긋남", func(s []seoulbus.Stop) { s[4] = busStop(5, 37.501, 127.0047, 445, false) }, false},
	}
	for _, c := range cases {
		stops := append([]seoulbus.Stop(nil), base...)
		c.edit(stops)
		pts, cum, m := matched(t, stops)
		if _, _, ok := busShape(pts, cum, m, stops, five); ok != c.ok {
			t.Errorf("%s: ok=%v", c.name, ok)
		}
	}
	if _, _, ok := busShape(nil, nil, nil, base, five); ok {
		t.Error("경로가 없는데 shape 가 나왔다")
	}
}

func TestShapeRowsSkipsRepeatedPoint(t *testing.T) {
	rows := shapeRows("S", []geo.Point{{Lat: 37.5, Lon: 127.0}, {Lat: 37.5000001, Lon: 127.0}, {Lat: 37.501, Lon: 127.0}})
	if len(rows) != 2 || rows[1][3] != "2" || rows[0][4] != "0.00" || rows[1][4] != "111.19" {
		t.Errorf("rows=%v", rows)
	}
	// 마지막 점이 빠지면 남는 점이 shape 길이를 갖는다(마지막 정류장 거리가 shape 길이를 넘지 않게).
	rows = shapeRows("S", []geo.Point{{Lat: 37.5, Lon: 127.0}, {Lat: 37.501, Lon: 127.0}, {Lat: 37.5010004, Lon: 127.0}})
	if len(rows) != 2 || rows[1][4] != "111.24" {
		t.Errorf("끝점: rows=%v", rows)
	}
}

// gpsX 에 TM 좌표가 오는 노선이 있다 — 경위도 범위 밖은 버린다. 연달아 온 같은 점은 하나만 남긴다.
func TestBusPathPointsDropsNonWGS84(t *testing.T) {
	got := busPathPoints([]seoulbus.PathPoint{{No: "1", Lon: "203389.569", Lat: "440840.9596"},
		{No: "2", Lon: "127.0", Lat: "37.5"}, {No: "3", Lon: "x", Lat: "37.5"}, {No: "4", Lon: "127.0", Lat: "37.5"}})
	if len(got) != 1 || got[0] != (geo.Point{Lat: 37.5, Lon: 127.0}) {
		t.Errorf("got=%v", got)
	}
}

// 배선: Build → zip 의 shapes.txt·trips.shape_id·stop_times.shape_dist_traveled.
func TestBuildWritesBusShapes(t *testing.T) {
	b := sampleRoute()
	b.Stops = loopStops()
	b.Path = loopPath()
	out := filepath.Join(t.TempDir(), "g.zip")
	rep, err := Build(out, []BusRoute{b}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NBusShapes != 2 || rep.NBusNoShapeDirections != 0 {
		t.Errorf("shape %d, 없는 방향 %d", rep.NBusShapes, rep.NBusNoShapeDirections)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	trips := readTable(t, zr, "trips.txt")
	if trips[0][5] != "shape_id" {
		t.Fatalf("trips 헤더=%v", trips[0])
	}
	shapeOf := map[string]string{}
	for _, r := range trips[1:] {
		shapeOf[r[2]] = r[5]
	}
	if shapeOf["B_100100047_T0"] != "BSH_100100047_0" || shapeOf["B_100100047_LAST0"] != "BSH_100100047_0" ||
		shapeOf["B_100100047_T1"] != "BSH_100100047_1" {
		t.Errorf("shape_id=%v", shapeOf)
	}
	var dists []string
	for _, r := range readTable(t, zr, "stop_times.txt")[1:] {
		if r[0] == "B_100100047_T1" {
			dists = append(dists, r[5])
		}
	}
	if len(dists) != 3 || dists[0] != "0.00" || dists[1] == "" || dists[2] == "" {
		t.Errorf("하행 shape_dist_traveled=%v", dists)
	}
	shapes := readTable(t, zr, "shapes.txt")
	n := 0
	for _, r := range shapes[1:] {
		if r[0] == "BSH_100100047_1" {
			n++
			if r[3] != fmt.Sprint(n) {
				t.Fatalf("shape_pt_sequence=%s want %d", r[3], n)
			}
		}
	}
	if n < 10 {
		t.Errorf("하행 shape 점 %d개", n)
	}

	// 노선 경로가 없으면 shapes.txt 를 쓰지 않고 shape_id 는 비운다.
	b.Path = nil
	rep, err = Build(out, []BusRoute{b}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	zr2, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr2.Close()
	for _, f := range zr2.File {
		if f.Name == "shapes.txt" {
			t.Error("경로가 없는데 shapes.txt 가 있다")
		}
	}
	if rep.NBusShapes != 0 || rep.NBusNoShapeDirections != 2 || readTable(t, zr2, "trips.txt")[1][5] != "" {
		t.Errorf("경로 없음: shape %d, 없는 방향 %d", rep.NBusShapes, rep.NBusNoShapeDirections)
	}
}

// OTP 는 shape_dist_traveled 자리가 정류장에서 150m 넘게 떨어지면 그 trip 을 직선으로 되돌린다. 어긋난 정류장(가운데·양 끝)도
// 경로선이 그 좌표를 지나고, 정류장별 거리는 shape 를 따라 잰 값과 맞아야 한다.
func TestBusShapeVisitsOffPathStops(t *testing.T) {
	stops := loopStops()
	stops[0] = busStop(1, 37.501, 126.9972, 0, false)   // 기점이 서쪽 250m
	stops[1] = busStop(2, 37.505, 126.9983, 445, false) // 가운데가 서쪽 150m
	stops[4] = busStop(5, 37.501, 127.0030, 445, false) // 종점이 동쪽 250m
	pts, cum, m := matched(t, stops)
	shape, dists, ok := busShape(pts, cum, m, stops, []int{0, 1, 2, 3, 4})
	if !ok {
		t.Fatal("shape 없음")
	}
	sc := geo.Cumulative(shape)
	for n, s := range stops {
		at := pointAt(shape, sc, dists[n])
		if d := geo.DistM(at, geo.Point{Lat: parseF(s.Lat), Lon: parseF(s.Lon)}); d > 30 {
			t.Errorf("S%d: shape 의 %.0fm 자리가 정류장에서 %.0fm 떨어져 있다", n+1, dists[n], d)
		}
		if n > 0 && dists[n] <= dists[n-1] {
			t.Errorf("S%d: 거리가 늘지 않는다 %v", n+1, dists)
		}
	}
	if math.Abs(dists[4]-sc[len(sc)-1]) > 0.5 {
		t.Errorf("마지막 정류장 %.1f, shape 길이 %.1f", dists[4], sc[len(sc)-1])
	}
}
