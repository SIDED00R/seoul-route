package build

import (
	"archive/zip"
	"encoding/csv"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/seoulbus"
)

func readTable(t *testing.T, zr *zip.ReadCloser, name string) [][]string {
	t.Helper()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		rows, err := csv.NewReader(rc).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	t.Fatalf("%s 없음", name)
	return nil
}

func sampleRoute() BusRoute {
	return BusRoute{
		Route: seoulbus.Route{ID: "100100047", Name: "271", Type: "3", StartName: "용마문화복지센터", EndName: "월드컵파크7단지",
			FirstBus: "20260912041000", LastBus: "20260912003000", TermMin: "6"},
		Stops: []seoulbus.Stop{
			{Seq: "1", StationID: "106000101", Name: "A", Lon: "127.095", Lat: "37.586", SectDist: "0", SectSpd: "0"},
			{Seq: "2", StationID: "106000100", Name: "B", Lon: "127.096", Lat: "37.589", SectDist: "360", SectSpd: "36"},
			{Seq: "3", StationID: "106000097", Name: "C", Lon: "127.098", Lat: "37.592", SectDist: "500", SectSpd: "0"},
		},
	}
}

// 배선을 타는 테스트: Build → zip → csv 를 다시 읽어 값으로 판정한다.
func TestBuildBusOnly(t *testing.T) {
	out := filepath.Join(t.TempDir(), "g.zip")
	rep, err := Build(out, []BusRoute{sampleRoute()}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NBusRoutes != 1 || rep.NBusStops != 3 {
		t.Fatalf("report=%+v", rep)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	st := readTable(t, zr, "stop_times.txt")
	// 360m@36km/h = 36s, 500m@18km/h(fallback) = 100s, 간선 정차 38s/구간 → 누적 0, 74, 212
	want := []string{"00:00:00", "00:01:14", "00:03:32"}
	var got []string
	for _, row := range st[1:] {
		if row[0] == "B_100100047_T" {
			got = append(got, row[1])
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("배차 trip stop_times=%v want %v", got, want)
	}
	fr := readTable(t, zr, "frequencies.txt")
	if len(fr) != 2 || fr[1][1] != "04:10:00" || fr[1][2] != "24:30:00" || fr[1][3] != "360" {
		t.Errorf("frequencies=%v (막차 00:30 은 자정 넘김이라 24:30:00 이어야 한다)", fr)
	}
	// 막차는 frequencies 의 배타적 end_time 에 걸리지 않도록 절대시각 trip 으로 따로 있어야 한다.
	tr := readTable(t, zr, "trips.txt")
	if len(tr) != 3 || tr[2][2] != "B_100100047_LAST" {
		t.Errorf("trips=%v (배차 trip + 막차 trip)", tr)
	}
	var lastTimes []string
	for _, row := range st[1:] {
		if row[0] == "B_100100047_LAST" {
			lastTimes = append(lastTimes, row[1])
		}
	}
	if strings.Join(lastTimes, ",") != "24:30:00,24:31:14,24:33:32" {
		t.Errorf("막차 trip stop_times=%v (막차 24:30:00 + 누적 소요)", lastTimes)
	}
	rt := readTable(t, zr, "routes.txt")
	if rt[1][0] != "B_100100047" || rt[1][4] != "3" {
		t.Errorf("routes=%v", rt[1])
	}
	// 간선(routeType 3)은 파랑·흰 글자. 헤더 이름은 서버 routestyle 이 키로 읽는다.
	if rt[0][5] != "route_color" || rt[0][6] != "route_text_color" || rt[1][5] != "0068B7" || rt[1][6] != "FFFFFF" {
		t.Errorf("노선색=%v (헤더 %v)", rt[1], rt[0])
	}
	sp := readTable(t, zr, "stops.txt")
	if len(sp) != 4 || !strings.HasPrefix(sp[1][0], "BS_") {
		t.Errorf("stops=%v", sp)
	}
	if rep.Routes[0].TravelSec != 212 {
		t.Errorf("report route=%+v", rep.Routes[0])
	}
}

// 회차 지점(transYn=Y)에서 상행·하행 trip 으로 나뉜다. 하행 frequencies 는 회차지 도착 시각만큼 늦게 시작하고,
// 하행 stop_times 는 회차지 0초 기준. 막차 trip 은 절대시각 그대로.
func TestBuildSplitsAtTurnaround(t *testing.T) {
	b := sampleRoute() // 간선: 정차 38초. A→B 36s, B→C 100s → 누적 0, 74, 212
	b.Stops = append(b.Stops, seoulbus.Stop{Seq: "4", StationID: "106000090", Name: "D", Lon: "127.099", Lat: "37.594",
		SectDist: "360", SectSpd: "36"}) // C→D 36+38 = 74 → 누적 286
	b.Stops[2].TransYn = "Y" // C 가 회차
	out := filepath.Join(t.TempDir(), "s.zip")
	rep, err := Build(out, []BusRoute{b}, nil, nil, nil, nil, nil)
	if err != nil || rep.Routes[0].TravelSec != 286 || rep.Routes[0].DroppedDirections != 0 {
		t.Fatalf("err=%v rep=%+v", err, rep.Routes)
	}
	zr, _ := zip.OpenReader(out)
	defer zr.Close()
	trips := map[string][]string{}
	for _, row := range readTable(t, zr, "trips.txt")[1:] {
		trips[row[2]] = row // trip_id → row(route_id, service_id, trip_id, headsign, direction_id)
	}
	if trips["B_100100047_T0"][3] != "C" || trips["B_100100047_T0"][4] != "0" ||
		trips["B_100100047_T1"][3] != "월드컵파크7단지" || trips["B_100100047_T1"][4] != "1" || len(trips) != 4 {
		t.Fatalf("상·하행 trip: %v", trips)
	}
	freq := map[string][]string{}
	for _, row := range readTable(t, zr, "frequencies.txt")[1:] {
		freq[row[0]] = row
	}
	if freq["B_100100047_T0"][1] != "04:10:00" || freq["B_100100047_T1"][1] != "04:13:32" { // 회차지 도착 212초 뒤
		t.Fatalf("하행 배차 시작은 첫차+회차 도착: %v", freq)
	}
	st := map[string][]string{}
	for _, row := range readTable(t, zr, "stop_times.txt")[1:] {
		st[row[0]] = append(st[row[0]], row[1]+"@"+row[4])
	}
	if strings.Join(st["B_100100047_T0"], ",") != "00:00:00@1,00:01:14@2,00:03:32@3" ||
		strings.Join(st["B_100100047_T1"], ",") != "00:00:00@3,00:01:14@4" ||
		strings.Join(st["B_100100047_LAST1"], ",") != "24:33:32@3,24:34:46@4" {
		t.Fatalf("stop_times: %v", st)
	}
}

// 서울 bbox 밖 정류장은 기록하지 않되 그 구간 주행시간은 누적된다. 안쪽 정류장이 2개 미만인 방향은 뺀다.
func TestBuildDropsStopsOutsideBBox(t *testing.T) {
	b := sampleRoute()
	b.Stops[1].Lon = "127.300" // B 가 경기(bbox 밖)
	out := filepath.Join(t.TempDir(), "b.zip")
	rep, err := Build(out, []BusRoute{b}, nil, nil, nil, nil, nil)
	if err != nil || rep.NBusStops != 2 || rep.Routes[0].TravelSec != 212 {
		t.Fatalf("err=%v rep=%+v stops=%d", err, rep.Routes, rep.NBusStops)
	}
	zr, _ := zip.OpenReader(out)
	defer zr.Close()
	var got []string
	for _, row := range readTable(t, zr, "stop_times.txt")[1:] {
		if row[0] == "B_100100047_T" {
			got = append(got, row[1]+"@"+row[4])
		}
	}
	if strings.Join(got, ",") != "00:00:00@1,00:03:32@3" {
		t.Fatalf("B 는 빠지고 C 의 누적시각은 유지: %v", got)
	}
	// 방향의 첫 정류장이 밖이면 배차 기준은 첫 안쪽 정류장: OTP 가 배차 trip 의 첫 stop_time 을 0 으로 정규화하므로
	// frequencies 시작을 그만큼(첫차+74초) 늦추고 상대시각은 0 부터 시작해야 한다.
	b = sampleRoute()
	b.Stops[0].Lon = "127.300" // A 가 밖, B·C 안
	out = filepath.Join(t.TempDir(), "a.zip")
	if _, err := Build(out, []BusRoute{b}, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	zr2, _ := zip.OpenReader(out)
	defer zr2.Close()
	fr := readTable(t, zr2, "frequencies.txt")
	if len(fr) != 2 || fr[1][1] != "04:11:14" || fr[1][2] != "24:31:14" {
		t.Fatalf("배차 시작·끝은 첫차·막차 + B 까지 누적(74초)여야: %v", fr)
	}
	got = nil
	for _, row := range readTable(t, zr2, "stop_times.txt")[1:] {
		if row[0] == "B_100100047_T" {
			got = append(got, row[1]+"@"+row[4])
		}
	}
	if strings.Join(got, ",") != "00:00:00@2,00:02:18@3" {
		t.Fatalf("첫 안쪽 정류장이 0초: %v", got)
	}

	b = sampleRoute()
	b.Stops[1].Lon, b.Stops[2].Lon = "127.300", "127.300" // B·C 밖 → 안쪽 1개뿐 → 방향 제거 → 노선 제외
	out = filepath.Join(t.TempDir(), "c.zip")
	rep, err = Build(out, []BusRoute{b}, nil, nil, nil, nil, nil)
	if err != nil || rep.Routes[0].DroppedDirections != 1 || rep.Routes[0].Skipped == "" || rep.NBusRoutes != 0 {
		t.Fatalf("전 방향이 빠지면 노선 제외로 보고: %v %+v n=%d", err, rep.Routes, rep.NBusRoutes)
	}
	zr3, _ := zip.OpenReader(out)
	defer zr3.Close()
	if rt := readTable(t, zr3, "routes.txt"); len(rt) != 1 {
		t.Fatalf("trip 없는 고아 route 행이 남으면 안 된다: %v", rt)
	}
}

// 정차시간은 노선유형별: 간선 38, 지선·미측정 유형 30, 마을·광역·공항 0.
func TestDwellSecByRouteType(t *testing.T) {
	cases := map[string]int{
		"3": DwellTrunkSec, "4": DwellBranchSec, "2": 0, "6": 0, "1": 0, " 8 ": DwellBranchSec, "": DwellBranchSec,
	}
	for typ, want := range cases {
		if got := dwellSec(typ); got != want {
			t.Errorf("routeType %q: got %d want %d", typ, got, want)
		}
	}
	b := sampleRoute()
	b.Route.Type = "2"
	rep, err := Build(filepath.Join(t.TempDir(), "m.zip"), []BusRoute{b}, nil, nil, nil, nil, nil)
	if err != nil || rep.Routes[0].TravelSec != 136 {
		t.Fatalf("마을버스는 정차 0 → 136 이어야: %v %+v", err, rep.Routes)
	}
}

// API 가 첫차·막차를 모르면 둘 다 자정을 준다 → 24시간 운행으로 만들면 안 되고 제외해야 한다.
func TestBuildSkipsRouteWithFirstEqualsLast(t *testing.T) {
	r := sampleRoute()
	r.Route.FirstBus, r.Route.LastBus = "20260913000000", "20260913000000"
	out := filepath.Join(t.TempDir(), "g.zip")
	rep, err := Build(out, []BusRoute{r}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NBusRoutes != 0 || rep.Routes[0].Skipped != "첫차·막차 시각 없음" {
		t.Fatalf("report=%+v", rep.Routes[0])
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if fr := readTable(t, zr, "frequencies.txt"); len(fr) != 1 {
		t.Fatalf("frequencies 에 00:00:00~24:00:00 행이 생기면 안 된다: %v", fr)
	}
}

func TestBuildSkipsRouteWithoutHeadway(t *testing.T) {
	r := sampleRoute()
	r.Route.TermMin = " "
	out := filepath.Join(t.TempDir(), "g.zip")
	rep, err := Build(out, []BusRoute{r}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NBusRoutes != 0 || rep.Routes[0].Skipped == "" {
		t.Fatalf("배차간격 없는 노선은 제외돼야 한다: %+v", rep.Routes[0])
	}
}
