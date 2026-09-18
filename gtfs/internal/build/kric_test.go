package build

import (
	"archive/zip"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/kric"
	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
)

func TestKricLineOf(t *testing.T) {
	cases := map[string]string{"RR_ACC1_S-1-KJ-1D": "KJ", "RR_ACC1_S-1-SB-1U": "SB", "RR_ACC1_S-1-01-1D": "01",
		"RR_ACC4_NAT0003_D": "", "B_100100047": ""}
	for id, want := range cases {
		if got := kricLineOf(id); got != want {
			t.Errorf("%s: got %q want %q", id, got, want)
		}
	}
}

func kjTimetable() *kric.Timetable {
	return &kric.Timetable{
		Lines: map[string]*kric.Line{
			"KJ": {Pilot: "KJ", Opr: "KR", Code: "K4", Name: "경의중앙", Stations: []kric.Station{
				{Code: "K110", Name: "용산", Order: 1, Lat: 37.5299, Lon: 126.9648},
				{Code: "K111", Name: "이촌", Order: 2, Lat: 37.5225, Lon: 126.9738},
				{Code: "K114", Name: "옥수", Order: 5, Lat: 37.5405, Lon: 127.0177},
				{Code: "K334", Name: "파주(두원대학)", Order: 50, Lat: 37.8330, Lon: 126.7970},
				{Code: "K999", Name: "없는역", Order: 60, Lat: 37.9, Lon: 126.9},
			}},
			"WS": {Pilot: "WS", Opr: "UI", Code: "UI", Name: "우이신설", HasSat: true, Stations: []kric.Station{
				{Code: "S110", Name: "북한산우이", Order: 1, Lat: 37.663, Lon: 127.012},
				{Code: "S111", Name: "솔밭공원", Order: 2, Lat: 37.658, Lon: 127.013},
			}},
		},
		Trains: []kric.Train{
			// 상행(순서 증가), 기점 도착 없음·종점 출발 없음, 없는역은 빠진다
			{Line: "KJ", Day: "8", No: "K5003", Org: "K110", Tmn: "K334", Stops: []kric.StopTime{
				{Code: "K110", Dep: "05:01:00"}, {Code: "K111", Arr: "05:04:00", Dep: "05:04:30"},
				{Code: "K999", Arr: "05:06:00", Dep: "05:06:30"}, {Code: "K334", Arr: "05:40:00"}}},
			// 하행 휴일 → 토요일 시각표가 없는 노선이라 SATSUN. 종점 코드가 역 목록에 없으면 마지막 정차 이름
			{Line: "KJ", Day: "9", No: "K5166", Org: "K126", Tmn: "K000", Stops: []kric.StopTime{
				{Code: "K114", Arr: "24:05:00", Dep: "24:05:30"}, {Code: "K110", Arr: "24:11:00"}}},
			// 정차 2개 미만(없는역 + 1개) → 뺀다
			{Line: "KJ", Day: "8", No: "K1", Org: "K110", Tmn: "K111", Stops: []kric.StopTime{
				{Code: "K999", Dep: "06:00:00"}, {Code: "K111", Arr: "06:03:00"}}},
			// 시각 역행 → 뺀다
			{Line: "KJ", Day: "8", No: "K2", Org: "K110", Tmn: "K114", Stops: []kric.StopTime{
				{Code: "K110", Dep: "07:00:00"}, {Code: "K111", Arr: "06:59:00", Dep: "07:03:30"},
				{Code: "K114", Arr: "07:10:00"}}},
			// 우이신설 토요일 → SAT
			{Line: "WS", Day: "7", No: "1508", Org: "S110", Tmn: "S111", Stops: []kric.StopTime{
				{Code: "S110", Dep: "06:00:00"}, {Code: "S111", Arr: "06:02:00"}}},
		},
	}
}

// 역 매칭: 괄호 앞 이름(파일럿 "옥수(경의중앙선)" ↔ 레일포털 "옥수", "파주(두원대학)" ↔ "파주"), 동명이면 최근접,
// 이름이 없으면 300m 안 최근접(이촌 → 파일럿 "이촌역"), 그것도 없으면 정차 제외.
func TestKricRows(t *testing.T) {
	pilot := map[string][]pilotStop{
		"KJ": {
			{ID: "RS_1008", Name: "옥수(경의중앙선)", Lat: 37.5405, Lon: 127.0177},
			{ID: "RS_1007", Name: "용산(경의중앙선)", Lat: 37.5299, Lon: 126.9648},
			{ID: "RS_1009", Name: "이촌역", Lat: 37.5226, Lon: 126.9739}, // 이름 불일치, 12m 거리
			{ID: "RS_1300", Name: "파주", Lat: 37.8330, Lon: 126.7970},
			{ID: "RS_1301", Name: "파주", Lat: 37.9, Lon: 126.5}, // 동명, 멀다
		},
		"WS": {{ID: "RS_4701", Name: "북한산우이", Lat: 37.663, Lon: 127.012},
			{ID: "RS_4702", Name: "솔밭공원", Lat: 37.658, Lon: 127.013}},
	}
	routes, trips, stopTimes, st := kricRows(kjTimetable(), pilot, map[string]string{"KJ": "경의중앙선"})
	if st.Trips != 3 || st.SkippedStops != 2 || st.SkippedTrips != 1 || st.NearestMatched != 1 || st.NonMonotonic != 1 {
		t.Fatalf("stats=%+v", st)
	}
	join := func(rows [][]string) string {
		var s []string
		for _, r := range rows {
			s = append(s, strings.Join(r, ","))
		}
		return strings.Join(s, "\n")
	}
	// 노선 코드별 색과 글자색이 붙는다. 우이신설은 노선 WS·기관 UI 라 기관 코드로 색을 찾으면 어긋난다(이슈 #71).
	if join(routes) != "K_KJ,A_KR,경의중앙선,경의중앙선,1,77C4A3,000000\nK_WS,A_UI,우이신설,우이신설,1,B0CE18,000000" {
		t.Fatalf("routes=\n%s", join(routes))
	}
	if join(trips) != "K_KJ,WEEKDAY,K_KJ_WEEKDAY_K5003,파주(두원대학),0\nK_KJ,SATSUN,K_KJ_SATSUN_K5166,용산,1\n"+
		"K_WS,SAT,K_WS_SAT_1508,솔밭공원,0" {
		t.Fatalf("trips=\n%s", join(trips))
	}
	want := "K_KJ_WEEKDAY_K5003,05:01:00,05:01:00,RS_1007,1\n" +
		"K_KJ_WEEKDAY_K5003,05:04:00,05:04:30,RS_1009,2\n" +
		"K_KJ_WEEKDAY_K5003,05:40:00,05:40:00,RS_1300,3\n" +
		"K_KJ_SATSUN_K5166,24:05:00,24:05:30,RS_1008,1\n" +
		"K_KJ_SATSUN_K5166,24:11:00,24:11:00,RS_1007,2\n" +
		"K_WS_SAT_1508,06:00:00,06:00:00,RS_4701,1\n" +
		"K_WS_SAT_1508,06:02:00,06:02:00,RS_4702,2"
	if join(stopTimes) != want {
		t.Fatalf("stop_times=\n%s\nwant\n%s", join(stopTimes), want)
	}
	if !kricNeedsSatSun(kjTimetable()) {
		t.Fatal("토요일 시각표 없는 노선이 있으면 SATSUN 이 필요하다")
	}
	ag := kricAgencies(kjTimetable())
	if len(ag) != 2 || ag[0][0] != "A_KR" || ag[0][1] != "코레일" || ag[1][0] != "A_UI" {
		t.Fatalf("agencies=%v", ag)
	}
}

// Build 는 레일포털 시각표가 있는 노선의 파일럿 route·trip 을 버리고 그 노선 정차역에 붙인 시각표 trip 을 넣는다.
// 다른 노선(신분당선)과 1~9호선 파일럿은 그대로다.
func TestBuildReplacesPilotKricTrips(t *testing.T) {
	sub := &ktdb.Subway{
		Routes: []ktdb.Row{{"route_id": "RR_ACC1_S-1-KJ-1D", "route_short_name": "경의중앙선", "route_long_name": "경의중앙선<하행>"},
			{"route_id": "RR_ACC1_S-1-KJ-1U", "route_short_name": "경의중앙선", "route_long_name": "경의중앙선<상행>"},
			{"route_id": "RR_ACC1_S-1-SB-1D", "route_short_name": "신분당선", "route_long_name": "신분당선<하행>"}},
		Trips: []ktdb.Row{{"route_id": "RR_ACC1_S-1-KJ-1D", "trip_id": "PKJ"},
			{"route_id": "RR_ACC1_S-1-KJ-1U", "trip_id": "PKJU"}, {"route_id": "RR_ACC1_S-1-SB-1D", "trip_id": "PSB"}},
		StopTimes: []ktdb.Row{
			pilotST("PKJ", "17:04:00", "RS_1007", "1"), pilotST("PKJ", "17:06:00", "RS_1009", "2"),
			pilotST("PKJU", "17:04:00", "RS_1009", "1"), pilotST("PKJU", "17:06:00", "RS_1007", "2"),
			pilotST("PSB", "06:00:00", "RS_4307", "1"), pilotST("PSB", "06:03:00", "RS_4308", "2"),
		},
		Stops: []ktdb.Row{
			{"stop_id": "RS_1007", "stop_name": "용산(경의중앙선)", "stop_lat": "37.5299", "stop_lon": "126.9648"},
			{"stop_id": "RS_1009", "stop_name": "이촌", "stop_lat": "37.5225", "stop_lon": "126.9738"},
			{"stop_id": "RS_4307", "stop_name": "강남(신분당선)", "stop_lat": "37.4966", "stop_lon": "127.0283"},
			{"stop_id": "RS_4308", "stop_name": "양재(신분당선)", "stop_lat": "37.4843", "stop_lon": "127.0344"},
		},
	}
	tt := &kric.Timetable{
		Lines: map[string]*kric.Line{"KJ": {Pilot: "KJ", Opr: "KR", Code: "K4", Name: "경의중앙", Stations: []kric.Station{
			{Code: "K110", Name: "용산", Order: 1, Lat: 37.5299, Lon: 126.9648},
			{Code: "K111", Name: "이촌", Order: 2, Lat: 37.5225, Lon: 126.9738}}}},
		Trains: []kric.Train{{Line: "KJ", Day: "9", No: "K5003", Org: "K110", Tmn: "K111", Stops: []kric.StopTime{
			{Code: "K110", Dep: "05:01:00"}, {Code: "K111", Arr: "05:04:00"}}}},
	}
	out := filepath.Join(t.TempDir(), "g.zip")
	rep, err := Build(out, []BusRoute{sampleRoute()}, sub, nil, nil, tt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NKricLines != 1 || rep.NKricTrips != 1 || rep.NPilotTripsReplaced != 2 || rep.NKricSkippedStops != 0 {
		t.Fatalf("report=%+v", rep)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var tripIDs, services []string
	for _, row := range readTable(t, zr, "trips.txt")[1:] {
		tripIDs = append(tripIDs, row[2])
		services = append(services, row[1])
	}
	got := strings.Join(tripIDs, ",")
	if strings.Contains(got, "PKJ") || !strings.Contains(got, "PSB") || !strings.Contains(got, "K_KJ_SATSUN_K5003") {
		t.Fatalf("파일럿 경의중앙 trip 은 빠지고 신분당선·시각표 trip 은 남아야: %v", tripIDs)
	}
	var ids []string
	pilotColors := map[string]string{}
	for _, r := range readTable(t, zr, "routes.txt")[1:] {
		ids = append(ids, r[0]+"/"+r[1]+"/"+r[2])
		pilotColors[r[0]] = r[5] + "/" + r[6]
	}
	// 시각표가 없어 파일럿으로 남는 노선도 코드로 색을 찾는다(이슈 #71)
	if pilotColors["RR_ACC1_S-1-SB-1D"] != "D4003B/FFFFFF" {
		t.Errorf("신분당선 파일럿 색=%q", pilotColors["RR_ACC1_S-1-SB-1D"])
	}
	all := strings.Join(ids, ",")
	if strings.Contains(all, "RR_ACC1_S-1-KJ") || !strings.Contains(all, "K_KJ/A_KR/경의중앙선") ||
		!strings.Contains(all, "RR_ACC1_S-1-SB") {
		t.Fatalf("routes=%v", ids)
	}
	cal := readTable(t, zr, "calendar.txt")
	if len(cal) != 6 || cal[5][0] != "SATSUN" || cal[5][6] != "1" || cal[5][7] != "1" || cal[5][5] != "0" {
		t.Fatalf("calendar=%v", cal)
	}
	if !strings.Contains(strings.Join(services, ","), "SATSUN") {
		t.Fatalf("services=%v", services)
	}
	ag := readTable(t, zr, "agency.txt")
	if len(ag) != 4 || ag[3][0] != "A_KR" {
		t.Fatalf("agency=%v", ag)
	}
	var rows int
	for _, r := range readTable(t, zr, "stop_times.txt")[1:] {
		if strings.HasPrefix(r[0], "K_KJ_") {
			rows++
		}
	}
	if rows != 2 {
		t.Fatalf("시각표 stop_times %d", rows)
	}
}
