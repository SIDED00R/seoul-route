package build

import (
	"archive/zip"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
	"github.com/SIDED00R/seoul-route/gtfs/internal/osm"
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
	parents, parentOf := stationGroups(stops)
	transfers := []ktdb.Row{
		{"from_stop_id": "RS_2", "to_stop_id": "RS_1", "transfer_type": "2", "min_transfer_time": "178"}}
	entrances, pathways, st := stationPathways(stops, parents, parentOf, transfers, nil)
	if len(entrances) != 4 || st.FarPairs != 0 || st.FallbackStns != 2 || st.RealEntranceStns != 0 {
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
	parents, parentOf := stationGroups(stops)
	if parentOf["RS_D"] == parentOf["RS_E"] {
		t.Fatalf("1.1km 떨어진 도봉산은 부모가 갈려야: %v", parentOf)
	}
	transfers := []ktdb.Row{
		{"from_stop_id": "RS_D", "to_stop_id": "RS_E", "transfer_type": "2", "min_transfer_time": "60"},
		{"from_stop_id": "RS_E", "to_stop_id": "RS_D", "transfer_type": "2", "min_transfer_time": "60"},
	}
	_, pathways, _ := stationPathways(stops, parents, parentOf, transfers, nil)
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
	parents, parentOf := stationGroups(stops)
	if parentOf["RS_1"] != parentOf["RS_2"] {
		t.Fatalf("중심 기준 800m 안이라 같은 부모여야: %v", parentOf)
	}
	entrances, pathways, st := stationPathways(stops, parents, parentOf, nil, nil)
	if st.FarPairs != 1 || len(entrances) != 2 || len(pathways) != 4 {
		t.Fatalf("통로는 출입구 4개뿐, 승강장 쌍은 상한 초과 1건: st=%+v entrances=%d pathways=%v", st, len(entrances), pathways)
	}
	for _, p := range pathways {
		if strings.HasPrefix(p[0], "PW_") {
			t.Fatalf("684m 쌍에 통로가 생기면 안 된다: %v", p)
		}
	}
}

// OSM 출입구가 어느 승강장 350m 안에 있으면 그 좌표에 출입구를 두고 승강장마다 진입(60+거리)·이탈(30+거리) 통로를 잇는다.
// 단일 승강장 역도 포함. 350m 밖 출입구는 무시(경계: 300m 매칭·400m 무시). 출구 번호는 숫자순, 숫자가 아닌 ref 는 OSM name,
// ref 가 비면 name 을 무시하고 "출입구". 출입구가 없는 다승강장 역은 승강장 좌표 폴백.
func TestStationPathwaysUsesOSMEntrances(t *testing.T) {
	stops := []ktdb.Row{
		{"stop_id": "RS_1", "stop_name": "강남(2호선)", "stop_lat": "37.4979", "stop_lon": "127.0276"},
		{"stop_id": "RS_2", "stop_name": "강남(신분당선)", "stop_lat": "37.4966", "stop_lon": "127.0283"},
		{"stop_id": "RS_3", "stop_name": "역삼(2호선)", "stop_lat": "37.5006", "stop_lon": "127.0364"},
		{"stop_id": "RS_4", "stop_name": "서울(1호선)", "stop_lat": "37.5546", "stop_lon": "126.9722"},
		{"stop_id": "RS_5", "stop_name": "서울(4호선)", "stop_lat": "37.5530", "stop_lon": "126.9720"},
	}
	parents, parentOf := stationGroups(stops)
	ents := []osm.Entrance{
		{Lat: 37.4980, Lon: 127.0290, Ref: "10"},                       // 가장 가까운 승강장 강남(2호선)에서 124m
		{Lat: 37.4968, Lon: 127.0272, Ref: "2"},                        // 강남(신분당선)에서 100m
		{Lat: 37.4940, Lon: 127.0240, Ref: "9"},                        // 가장 가까운 승강장까지 477m → 무시
		{Lat: 37.4939, Lon: 127.0283, Ref: "11"},                       // 강남(신분당선)에서 300m → 매칭(250m 면 탈락)
		{Lat: 37.4930, Lon: 127.0283, Ref: "12"},                       // 400m → 무시(350m 밖)
		{Lat: 37.5008, Lon: 127.0370, Ref: ""},                         // 역삼 57m, 번호 없음
		{Lat: 37.5004, Lon: 127.0360, Ref: "엘리베이터", Name: "역삼(엘리베이터)"}, // 숫자가 아닌 ref → name
		{Lat: 37.5010, Lon: 127.0368, Ref: "", Name: "3"},              // ref 없음 → name 을 보지 않는다
	}
	entrances, pathways, st := stationPathways(stops, parents, parentOf, nil, ents)
	if st.RealEntranceStns != 2 || st.FallbackStns != 1 {
		t.Fatalf("강남·역삼은 OSM 출입구, 서울은 폴백: %+v", st)
	}
	var names []string
	for _, e := range entrances {
		names = append(names, e[0]+"|"+e[1]+"|"+e[4]+"|"+e[5])
	}
	want := []string{
		"EN_ST_강남_1|강남 2번 출구|2|ST_강남", "EN_ST_강남_2|강남 10번 출구|2|ST_강남", // 숫자순(2 < 10 < 11)
		"EN_ST_강남_3|강남 11번 출구|2|ST_강남",
		"EN_RS_4|서울(1호선)|2|ST_서울", "EN_RS_5|서울(4호선)|2|ST_서울",
		// 숫자 ref 가 없으니 ref 문자열순: "" < "엘리베이터" → 번호 없는 둘이 먼저
		"EN_ST_역삼_1|역삼 출입구|2|ST_역삼", "EN_ST_역삼_2|역삼 출입구|2|ST_역삼", "EN_ST_역삼_3|역삼(엘리베이터)|2|ST_역삼",
	}
	if strings.Join(names, "\n") != strings.Join(want, "\n") {
		t.Fatalf("entrances=\n%s\nwant\n%s", strings.Join(names, "\n"), strings.Join(want, "\n"))
	}
	if entrances[0][2] != "37.496800" || entrances[0][3] != "127.027200" {
		t.Fatalf("출입구 좌표는 OSM 값: %v", entrances[0])
	}
	// 강남 2번 출구 ↔ 강남(2호선): 직선 127m → 진입 60+127=187, 이탈 30+127=157
	var in, out string
	for _, p := range pathways {
		switch p[0] {
		case "PWI_EN_ST_강남_1_RS_1":
			in = p[1] + ">" + p[2] + "," + p[4] + "," + p[5]
		case "PWO_EN_ST_강남_1_RS_1":
			out = p[1] + ">" + p[2] + "," + p[4] + "," + p[5]
		}
	}
	if in != "EN_ST_강남_1>RS_1,0,187" || out != "RS_1>EN_ST_강남_1,0,157" {
		t.Fatalf("진입·이탈 통로: in=%q out=%q", in, out)
	}
	// 강남 통로 = 출입구 3 × 승강장 2 × 2방향 + 승강장 쌍 1, 역삼 출입구 3 × 2, 서울 폴백 4 + 쌍 1
	if len(pathways) != 12+1+6+4+1 || st.NoEntrancePlatforms != 0 {
		t.Fatalf("pathways %d st=%+v", len(pathways), st)
	}
}

// 신촌: 두 승강장이 684m 떨어져 부모 평균 좌표에서는 실제 출구가 250m 밖(302m)이지만 각 승강장에서는 40~60m.
// 승강장 기준으로 매칭해 붙이고, 출입구↔승강장 통로는 500m 안 승강장에만(2호선 출구가 경의중앙선 승강장과 이어지지 않는다).
func TestStationPathwaysMatchesEntrancesByPlatform(t *testing.T) {
	stops := []ktdb.Row{
		{"stop_id": "RS_1", "stop_name": "신촌(2호선)", "stop_lat": "37.555172", "stop_lon": "126.937004"},
		{"stop_id": "RS_2", "stop_name": "신촌(경의중앙선)", "stop_lat": "37.559776", "stop_lon": "126.942150"},
	}
	parents, parentOf := stationGroups(stops)
	ents := []osm.Entrance{
		{Lat: 37.555500, Lon: 126.936800, Ref: "3"}, // 2호선 승강장 40m, 부모에서 300m 넘음
		{Lat: 37.559300, Lon: 126.942300, Ref: "1"}, // 경의중앙선 승강장 55m
	}
	entrances, pathways, st := stationPathways(stops, parents, parentOf, nil, ents)
	if st.RealEntranceStns != 1 || st.FallbackStns != 0 || st.NoEntrancePlatforms != 0 || len(entrances) != 2 {
		t.Fatalf("승강장 기준으로 신촌에 출입구 2개가 붙어야: st=%+v entrances=%v", st, entrances)
	}
	var ids []string
	for _, p := range pathways {
		ids = append(ids, p[0])
	}
	// 1번 출구(경의중앙선 옆)는 경의중앙선 승강장에만, 3번 출구는 2호선 승강장에만. 승강장 쌍 통로는 684m 라 없음.
	want := "PWI_EN_ST_신촌_1_RS_2,PWO_EN_ST_신촌_1_RS_2,PWI_EN_ST_신촌_2_RS_1,PWO_EN_ST_신촌_2_RS_1"
	if strings.Join(ids, ",") != want {
		t.Fatalf("pathways=%v", ids)
	}
	// 경의중앙선 출구가 없으면 그 승강장은 통로를 못 받아 고립 수로 잡힌다.
	_, _, st = stationPathways(stops, parents, parentOf, nil, ents[:1])
	if st.NoEntrancePlatforms != 1 {
		t.Fatalf("출입구 통로 없는 승강장 1: %+v", st)
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
	rep, err := Build(out, []BusRoute{sampleRoute()}, sub, nil)
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
