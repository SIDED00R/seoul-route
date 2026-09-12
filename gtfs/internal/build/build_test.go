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
	rep, err := Build(out, []BusRoute{sampleRoute()}, nil)
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
	sp := readTable(t, zr, "stops.txt")
	if len(sp) != 4 || !strings.HasPrefix(sp[1][0], "BS_") {
		t.Errorf("stops=%v", sp)
	}
	if rep.Routes[0].TravelSec != 212 {
		t.Errorf("report route=%+v", rep.Routes[0])
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
	rep, err := Build(filepath.Join(t.TempDir(), "m.zip"), []BusRoute{b}, nil)
	if err != nil || rep.Routes[0].TravelSec != 136 {
		t.Fatalf("마을버스는 정차 0 → 136 이어야: %v %+v", err, rep.Routes)
	}
}

// API 가 첫차·막차를 모르면 둘 다 자정을 준다 → 24시간 운행으로 만들면 안 되고 제외해야 한다.
func TestBuildSkipsRouteWithFirstEqualsLast(t *testing.T) {
	r := sampleRoute()
	r.Route.FirstBus, r.Route.LastBus = "20260913000000", "20260913000000"
	out := filepath.Join(t.TempDir(), "g.zip")
	rep, err := Build(out, []BusRoute{r}, nil)
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
	rep, err := Build(out, []BusRoute{r}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NBusRoutes != 0 || rep.Routes[0].Skipped == "" {
		t.Fatalf("배차간격 없는 노선은 제외돼야 한다: %+v", rep.Routes[0])
	}
}
