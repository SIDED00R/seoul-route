package build

import (
	"archive/zip"
	"path/filepath"
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
	"github.com/SIDED00R/seoul-route/gtfs/internal/seoulmetro"
)

func TestLine2LoopBlocks(t *testing.T) {
	names := map[string]string{"HY": "한양대(2호선)", "TS": "뚝섬(2호선)", "SS": "성수(2호선)", "KD": "건대입구(2호선)",
		"GU": "구의(광진구청)(2호선)", "YD": "용답(2호선)"}
	var trips, st [][]string
	add := func(id, service string, stops ...[2]string) { // [stop_id, 시각]
		trips = append(trips, []string{"M_2", service, id, "성수", "0"})
		for _, s := range stops {
			st = append(st, []string{id, s[1], s[1], s[0], ""})
		}
	}
	// A 뚝섬→성수 08:00:30 도착 → B 08:01:30 성수→건대입구(이어짐). C 08:02:00 도 같은 방향이지만 B 가 먼저라 짝 없음.
	add("A", "WEEKDAY", [2]string{"HY", "07:57:00"}, [2]string{"TS", "07:59:00"}, [2]string{"SS", "08:00:30"})
	add("B", "WEEKDAY", [2]string{"SS", "08:01:30"}, [2]string{"KD", "08:03:00"}, [2]string{"TS", "09:29:00"},
		[2]string{"SS", "09:30:30"})
	add("C", "WEEKDAY", [2]string{"SS", "08:02:00"}, [2]string{"KD", "08:03:30"})
	// B 는 한 바퀴 돌아 09:30:30 성수 도착 → K 09:31:00(이어짐, A·B·K 가 한 block).
	add("K", "WEEKDAY", [2]string{"SS", "09:31:00"}, [2]string{"KD", "09:32:30"})
	// D 건대입구→성수 08:10:00 → E 08:11:00 성수→뚝섬(이어짐). F 08:10:30 은 성수→건대입구라 반대 방향.
	add("D", "WEEKDAY", [2]string{"GU", "08:08:00"}, [2]string{"KD", "08:09:00"}, [2]string{"SS", "08:10:00"})
	add("E", "WEEKDAY", [2]string{"SS", "08:11:00"}, [2]string{"TS", "08:12:30"})
	add("F", "WEEKDAY", [2]string{"SS", "08:10:30"}, [2]string{"KD", "08:12:00"})
	// G 는 3분 안에 이어지는 출발이 없다(H 는 4분 뒤) → 짝 없음.
	add("G", "WEEKDAY", [2]string{"TS", "11:59:00"}, [2]string{"SS", "12:00:00"})
	add("H", "WEEKDAY", [2]string{"SS", "12:04:00"}, [2]string{"KD", "12:05:30"})
	// S 는 토요일 도착이라 평일 출발 B 와 잇지 않는다. Y 는 지선(용답)에서 들어와 대상이 아니다.
	add("S", "SAT", [2]string{"TS", "07:59:30"}, [2]string{"SS", "08:01:00"})
	add("Y", "WEEKDAY", [2]string{"YD", "08:00:00"}, [2]string{"SS", "08:01:00"})

	blocks, stats := line2LoopBlocks(trips, st, names)
	want := map[string]string{"A": "B_A", "B": "B_A", "K": "B_A", "D": "B_D", "E": "B_D"}
	if len(blocks) != len(want) {
		t.Fatalf("blocks=%v", blocks)
	}
	for trip, b := range want {
		if blocks[trip] != b {
			t.Errorf("%s: block %q want %q", trip, blocks[trip], b)
		}
	}
	if stats != (loopStats{Links: 3, Blocks: 2, Unpaired: 2}) {
		t.Errorf("stats=%+v", stats)
	}
}

// 배선: Build 가 2호선 시각표 trip 에 block_id 를 붙여 trips.txt 에 쓰고 보고서에 센다.
func TestBuildWritesLoopBlocks(t *testing.T) {
	sub := &ktdb.Subway{
		Stops: []ktdb.Row{
			{"stop_id": "RS_ACC1_S-1-0210", "stop_name": "뚝섬(2호선)", "stop_lat": "37.5474", "stop_lon": "127.0474"},
			{"stop_id": "RS_ACC1_S-1-0211", "stop_name": "성수(2호선)", "stop_lat": "37.5446", "stop_lon": "127.0560"},
			{"stop_id": "RS_ACC1_S-1-0212", "stop_name": "건대입구(2호선)", "stop_lat": "37.5404", "stop_lon": "127.0692"},
		},
	}
	metro := &seoulmetro.Timetable{Trains: []seoulmetro.Train{
		{Line: "2", Day: "DAY", Code: "2048", Dir: "OUT", Dest: "성수", Stops: []seoulmetro.StopTime{
			{Code: "0210", Dep: "07:59:00"}, {Code: "0211", Arr: "08:00:30"}}},
		{Line: "2", Day: "DAY", Code: "2102", Dir: "OUT", Dest: "성수", Stops: []seoulmetro.StopTime{
			{Code: "0211", Dep: "08:01:30"}, {Code: "0212", Arr: "08:03:00"}}},
	}}
	out := filepath.Join(t.TempDir(), "g.zip")
	rep, err := Build(out, nil, sub, nil, metro, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NLoopLinks != 1 || rep.NLoopBlocks != 1 || rep.NLoopUnpaired != 0 {
		t.Fatalf("report=%+v", rep)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	tr := readTable(t, zr, "trips.txt")
	if tr[0][6] != "block_id" {
		t.Fatalf("header=%v", tr[0])
	}
	got := map[string]string{}
	for _, r := range tr[1:] {
		got[r[2]] = r[6]
	}
	if got["M_2_DAY_2048"] != "B_M_2_DAY_2048" || got["M_2_DAY_2102"] != "B_M_2_DAY_2048" {
		t.Errorf("block_id=%v", got)
	}
}
