package build

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/kric"
	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
	"github.com/SIDED00R/seoul-route/gtfs/internal/seoulmetro"
)

// 구간속도 3km/h(MinSectSpeedKmh 미만)는 폴백 18km/h 로 계산하고 보고서에 센다. 5km/h 는 그대로 쓴다.
func TestBuildSlowSectionUsesFallbackSpeed(t *testing.T) {
	b := sampleRoute()
	b.Stops[1].SectSpd = "3" // 360m@18km/h = 72s + 정차 38 = 110
	b.Stops[2].SectSpd = "5" // 500m@5km/h = 360s + 정차 38 = 398 → 누적 508
	out := filepath.Join(t.TempDir(), "g.zip")
	rep, err := Build(out, []BusRoute{b}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NBusSlowSections != 1 || rep.Routes[0].SlowSections != 1 || rep.Routes[0].TravelSec != 508 {
		t.Fatalf("slow=%d route=%+v", rep.NBusSlowSections, rep.Routes[0])
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var got []string
	for _, row := range readTable(t, zr, "stop_times.txt")[1:] {
		if row[0] == "B_100100047_T" {
			got = append(got, row[1])
		}
	}
	if strings.Join(got, ",") != "00:00:00,00:01:50,00:08:28" {
		t.Fatalf("stop_times=%v", got)
	}
}

// 속도 결측(0)은 폴백을 쓰지만 저속 구간으로 세지 않는다.
func TestBuildMissingSpeedIsNotCountedSlow(t *testing.T) {
	rep, err := Build(filepath.Join(t.TempDir(), "g.zip"), []BusRoute{sampleRoute()}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NBusSlowSections != 0 {
		t.Fatalf("slow=%d", rep.NBusSlowSections)
	}
}

func pilotLines() *ktdb.Subway {
	return &ktdb.Subway{
		Routes: []ktdb.Row{
			{"route_id": "RR_ACC1_S-1-03-1D", "route_short_name": "3호선", "route_long_name": "3호선<하행>"},
			{"route_id": "RR_ACC1_S-1-KJ-1D", "route_short_name": "경의중앙선", "route_long_name": "경의중앙선<하행>"}},
		Trips: []ktdb.Row{{"route_id": "RR_ACC1_S-1-03-1D", "trip_id": "P3"},
			{"route_id": "RR_ACC1_S-1-KJ-1D", "trip_id": "PKJ"}},
		StopTimes: []ktdb.Row{
			pilotST("P3", "17:04:00", "RS_ACC1_S-1-0301", "1"), pilotST("P3", "17:06:00", "RS_ACC1_S-1-0302", "2"),
			pilotST("PKJ", "17:04:00", "RS_1007", "1"), pilotST("PKJ", "17:06:00", "RS_1009", "2"),
		},
		Stops: []ktdb.Row{
			{"stop_id": "RS_ACC1_S-1-0301", "stop_name": "A(3호선)", "stop_lat": "37.5193", "stop_lon": "127.0532"},
			{"stop_id": "RS_ACC1_S-1-0302", "stop_name": "B(3호선)", "stop_lat": "37.5172", "stop_lon": "127.0412"},
			{"stop_id": "RS_ACC1_S-1-0701", "stop_name": "C(7호선)", "stop_lat": "37.5193", "stop_lon": "127.0632"},
			{"stop_id": "RS_ACC1_S-1-0702", "stop_name": "D(7호선)", "stop_lat": "37.5172", "stop_lon": "127.0712"},
			{"stop_id": "RS_1007", "stop_name": "용산(경의중앙선)", "stop_lat": "37.5299", "stop_lon": "126.9648"},
			{"stop_id": "RS_1009", "stop_name": "이촌", "stop_lat": "37.5225", "stop_lon": "126.9738"},
		},
	}
}

// 서울교통공사 시각표에 파일럿이 가진 호선(3호선)의 열차가 없으면 빌드가 실패한다.
func TestBuildFailsWhenMetroTimetableMissesLine(t *testing.T) {
	metro := &seoulmetro.Timetable{Trains: []seoulmetro.Train{
		{Line: "7", Day: "DAY", Code: "7006", Dir: "DOWN", Dest: "온수", Stops: []seoulmetro.StopTime{
			{Code: "0701", Dep: "05:39:00"}, {Code: "0702", Arr: "05:41:30"}}},
	}}
	_, err := Build(filepath.Join(t.TempDir(), "g.zip"), nil, pilotLines(), nil, metro, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "3호선") {
		t.Fatalf("err=%v", err)
	}
}

// 레일포털 역 파일에는 있는 노선(KJ)의 열차가 시각표에 없으면 빌드가 실패한다.
func TestBuildFailsWhenKricTimetableMissesLine(t *testing.T) {
	tt := &kric.Timetable{
		Lines: map[string]*kric.Line{"KJ": {Pilot: "KJ", Opr: "KR", Code: "K4", Name: "경의중앙", Stations: []kric.Station{
			{Code: "K110", Name: "용산", Order: 1, Lat: 37.5299, Lon: 126.9648},
			{Code: "K111", Name: "이촌", Order: 2, Lat: 37.5225, Lon: 126.9738}}}},
	}
	_, err := Build(filepath.Join(t.TempDir(), "g.zip"), nil, pilotLines(), nil, nil, tt, nil)
	if err == nil || !strings.Contains(err.Error(), "KJ") {
		t.Fatalf("err=%v", err)
	}
}

// 검증에 걸려 실패한 빌드는 기존 산출물을 그대로 두고 임시 파일도 남기지 않는다.
func TestBuildFailureKeepsPreviousOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "g.zip")
	if _, err := Build(out, []BusRoute{sampleRoute()}, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	metro := &seoulmetro.Timetable{Trains: []seoulmetro.Train{
		{Line: "7", Day: "DAY", Code: "7006", Dir: "DOWN", Dest: "온수", Stops: []seoulmetro.StopTime{
			{Code: "0701", Dep: "05:39:00"}, {Code: "0702", Arr: "05:41:30"}}},
	}}
	if _, err := Build(out, nil, pilotLines(), nil, metro, nil, nil); err == nil {
		t.Fatal("실패해야 한다")
	}
	after, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("실패한 빌드가 기존 산출물을 바꿨다")
	}
	if _, err := os.Stat(out + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("임시 파일이 남았다: %v", err)
	}
}
