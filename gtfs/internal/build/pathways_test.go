package build

import (
	"archive/zip"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
)

// 자식 stop 이 2개 이상인 역만: 승강장마다 출입구 + 진입 120/이탈 60 단방향 통로, 승강장 쌍마다 양방향 walkway.
// 환승 통과시간은 transfers.txt 값이 있으면 그 값(역방향 행이라도), 없으면 거리÷1.0m/s + 60초. 단일 승강장 역은 없음.
func TestStationPathways(t *testing.T) {
	stops := []ktdb.Row{
		{"stop_id": "RS_2", "stop_name": "강남(신분당선)", "stop_lat": "37.4966", "stop_lon": "127.0283"},
		{"stop_id": "RS_1", "stop_name": "강남(2호선)", "stop_lat": "37.4979", "stop_lon": "127.0276"},
		{"stop_id": "RS_3", "stop_name": "역삼(2호선)", "stop_lat": "37.5006", "stop_lon": "127.0364"},
		{"stop_id": "RS_4", "stop_name": "서울(1호선)", "stop_lat": "37.5546", "stop_lon": "126.9722"},
		{"stop_id": "RS_5", "stop_name": "서울(4호선)", "stop_lat": "37.5530", "stop_lon": "126.9720"},
	}
	_, parentOf := stationGroups(stops)
	transfers := []ktdb.Row{
		{"from_stop_id": "RS_2", "to_stop_id": "RS_1", "transfer_type": "2", "min_transfer_time": "178"}}
	entrances, pathways, far := stationPathways(stops, parentOf, transfers)
	if len(entrances) != 4 || far != 0 {
		t.Fatalf("강남 2 + 서울 2 출입구여야(역삼은 단일 승강장): %v", entrances)
	}
	if strings.Join(entrances[0], ",") != "EN_RS_1,강남(2호선),37.4979,127.0276,2,ST_강남" {
		t.Fatalf("출입구는 승강장과 같은 좌표·같은 부모: %v", entrances[0])
	}
	joined := make([]string, len(pathways))
	for i, p := range pathways {
		joined[i] = strings.Join(p, ",")
	}
	want := []string{
		"PWI_RS_1,EN_RS_1,RS_1,1,0,120", "PWO_RS_1,RS_1,EN_RS_1,1,0,60",
		"PWI_RS_2,EN_RS_2,RS_2,1,0,120", "PWO_RS_2,RS_2,EN_RS_2,1,0,60",
		"PW_RS_1_RS_2,RS_1,RS_2,1,1,178", // transfers 의 178초
		"PWI_RS_4,EN_RS_4,RS_4,1,0,120", "PWO_RS_4,RS_4,EN_RS_4,1,0,60",
		"PWI_RS_5,EN_RS_5,RS_5,1,0,120", "PWO_RS_5,RS_5,EN_RS_5,1,0,60",
		"PW_RS_4_RS_5,RS_4,RS_5,1,1,238", // 직선 178m → 178/1.0 + 60
	}
	if strings.Join(joined, "\n") != strings.Join(want, "\n") {
		t.Fatalf("pathways=\n%s\nwant\n%s", strings.Join(joined, "\n"), strings.Join(want, "\n"))
	}
}

// 승강장 3개 역은 쌍 3개(a-b, a-c, b-c 순). 부모역이 갈린 transfers 행은 통로 없이 개수만 센다.
func TestStationPathwaysThreePlatformsAndUnpaired(t *testing.T) {
	stops := []ktdb.Row{
		{"stop_id": "RS_C", "stop_name": "고속터미널(7호선)", "stop_lat": "37.5048", "stop_lon": "127.0056"},
		{"stop_id": "RS_A", "stop_name": "고속터미널(3호선)", "stop_lat": "37.5049", "stop_lon": "127.0049"},
		{"stop_id": "RS_B", "stop_name": "고속터미널(9호선)", "stop_lat": "37.5045", "stop_lon": "127.0044"},
		{"stop_id": "RS_D", "stop_name": "도봉산(1호선)", "stop_lat": "37.6791", "stop_lon": "127.0455"},
		{"stop_id": "RS_E", "stop_name": "도봉산(7호선)", "stop_lat": "37.6891", "stop_lon": "127.0466"},
	}
	_, parentOf := stationGroups(stops)
	if parentOf["RS_D"] == parentOf["RS_E"] {
		t.Fatalf("1.1km 떨어진 도봉산은 부모가 갈려야: %v", parentOf)
	}
	transfers := []ktdb.Row{
		{"from_stop_id": "RS_D", "to_stop_id": "RS_E", "transfer_type": "2", "min_transfer_time": "60"},
		{"from_stop_id": "RS_E", "to_stop_id": "RS_D", "transfer_type": "2", "min_transfer_time": "60"},
	}
	_, pathways, _ := stationPathways(stops, parentOf, transfers)
	var pairs []string
	for _, p := range pathways {
		if strings.HasPrefix(p[0], "PW_") {
			pairs = append(pairs, p[0])
		}
	}
	if strings.Join(pairs, ",") != "PW_RS_A_RS_B,PW_RS_A_RS_C,PW_RS_B_RS_C" {
		t.Fatalf("3승강장 쌍 3개여야: %v", pairs)
	}
	if n := unpairedTransfers(transfers, parentOf); n != 2 {
		t.Fatalf("도봉산 왕복 2행이 빠진 것으로 세어야: %d", n)
	}
}

// transfers 값이 없고 500m 를 넘는 쌍(신촌 2호선↔경의중앙선 684m)은 통로를 만들지 않고 개수만 센다. 출입구는 그대로.
func TestStationPathwaysSkipsFarFallbackPairs(t *testing.T) {
	stops := []ktdb.Row{
		{"stop_id": "RS_1", "stop_name": "신촌(2호선)", "stop_lat": "37.555172", "stop_lon": "126.937004"},
		{"stop_id": "RS_2", "stop_name": "신촌(경의중앙선)", "stop_lat": "37.559776", "stop_lon": "126.942150"},
	}
	_, parentOf := stationGroups(stops)
	if parentOf["RS_1"] != parentOf["RS_2"] {
		t.Fatalf("중심 기준 800m 안이라 같은 부모여야: %v", parentOf)
	}
	entrances, pathways, far := stationPathways(stops, parentOf, nil)
	if far != 1 || len(entrances) != 2 || len(pathways) != 4 {
		t.Fatalf("통로는 출입구 4개뿐, 승강장 쌍은 상한 초과 1건: far=%d entrances=%d pathways=%v", far, len(entrances), pathways)
	}
	for _, p := range pathways {
		if strings.HasPrefix(p[0], "PW_") {
			t.Fatalf("684m 쌍에 통로가 생기면 안 된다: %v", p)
		}
	}
}

// Build 가 출입구를 stops.txt 에, 통로를 pathways.txt 에 쓰고 보고서에 개수를 남긴다.
func TestBuildWritesPathways(t *testing.T) {
	sub := &ktdb.Subway{
		Routes: []ktdb.Row{{"route_id": "R1", "route_short_name": "2호선", "route_long_name": "서울2호선"}},
		Trips:  []ktdb.Row{{"route_id": "R1", "trip_id": "T1"}},
		StopTimes: []ktdb.Row{
			{"trip_id": "T1", "arrival_time": "05:00:00", "departure_time": "05:00:00", "stop_id": "RS_1", "stop_sequence": "1"},
			{"trip_id": "T1", "arrival_time": "05:02:00", "departure_time": "05:02:00", "stop_id": "RS_3", "stop_sequence": "2"},
		},
		Stops: []ktdb.Row{
			{"stop_id": "RS_1", "stop_name": "강남(2호선)", "stop_lat": "37.4979", "stop_lon": "127.0276"},
			{"stop_id": "RS_2", "stop_name": "강남(신분당선)", "stop_lat": "37.4966", "stop_lon": "127.0283"},
			{"stop_id": "RS_3", "stop_name": "역삼(2호선)", "stop_lat": "37.5006", "stop_lon": "127.0364"},
		},
	}
	out := filepath.Join(t.TempDir(), "g.zip")
	rep, err := Build(out, []BusRoute{sampleRoute()}, sub)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NPathways != 5 || rep.NEntrances != 2 {
		t.Fatalf("report=%+v", rep)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	pw := readTable(t, zr, "pathways.txt")
	header := "pathway_id,from_stop_id,to_stop_id,pathway_mode,is_bidirectional,traversal_time"
	if len(pw) != 6 || strings.Join(pw[0], ",") != header {
		t.Fatalf("pathways=%v", pw)
	}
	var entrances int
	for _, row := range readTable(t, zr, "stops.txt")[1:] {
		if row[4] == "2" {
			entrances++
			if !strings.HasPrefix(row[0], "EN_RS_") || row[5] != "ST_강남" {
				t.Fatalf("출입구 행=%v", row)
			}
		}
	}
	if entrances != 2 {
		t.Fatalf("stops.txt 출입구 %d개", entrances)
	}
}
