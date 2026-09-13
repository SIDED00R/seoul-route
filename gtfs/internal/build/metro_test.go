package build

import (
	"archive/zip"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
	"github.com/SIDED00R/seoul-route/gtfs/internal/seoulmetro"
)

func TestMetroReplacesRoute(t *testing.T) {
	cases := map[string]bool{
		"RR_ACC1_S-1-01-1D": true, "RR_ACC1_S-1-07-1U": true, "RR_ACC1_S-1-09-2D": true, "RR_ACC1_S-1-06-00": true,
		"RR_ACC1_S-1-SB-1D": false, "RR_ACC1_S-1-KJ-2U": false, "RR_ACC1_S-1-I1-1D": false, "B_100100047": false,
	}
	for id, want := range cases {
		if got := metroReplacesRoute(id); got != want {
			t.Errorf("%s: got %v want %v", id, got, want)
		}
	}
}

// 시각표 열차 → route(호선/급행)·trip(요일 service, 방향, 도착역)·stop_times(기점 도착=출발, 종점 출발=도착).
// 파일럿에 없는 역은 정차만 빼고, 정차가 2개 미만이면 열차를 뺀다.
func TestMetroRows(t *testing.T) {
	tt := &seoulmetro.Timetable{Trains: []seoulmetro.Train{
		{Line: "7", Day: "DAY", Code: "7006", Dir: "DOWN", Origin: "청담", Dest: "온수", Stops: []seoulmetro.StopTime{
			{Code: "2731", Dep: "05:39:00"}, {Code: "2732", Arr: "05:41:30", Dep: "05:41:50"}, {Code: "2733", Arr: "05:43:20"}}},
		{Line: "9", Day: "END", Code: "9502", Dir: "UP", Express: true, Dest: "중앙보훈병원", Stops: []seoulmetro.StopTime{
			{Code: "4101", Dep: "23:58:00"}, {Code: "9999", Arr: "24:01:00", Dep: "24:01:30"}, {Code: "4103", Arr: "24:03:00"}}},
		{Line: "2", Day: "SAT", Code: "2001", Dir: "IN", Stops: []seoulmetro.StopTime{
			{Code: "0222", Dep: "06:00:00"}, {Code: "9998", Arr: "06:02:00"}}},
		// 까치산: 코드 0200 은 파일럿에 없지만 이름 "까치산(2호선)" 으로 찾는다.
		{Line: "2", Day: "DAY", Code: "2300", Dir: "OUT", Dest: "까치산", Stops: []seoulmetro.StopTime{
			{Code: "0222", Arr: "07:00:00", Dep: "07:00:30"}, {Code: "0200", Name: "까치산", Arr: "07:05:00", Dep: "07:05:00"}}},
		// 시각 역행(둘째 정차 도착 < 첫 정차 출발): 열차 통째로 제외.
		{Line: "9", Day: "SAT", Code: "C9199", Dir: "DOWN", Dest: "동작", Stops: []seoulmetro.StopTime{
			{Code: "4101", Dep: "23:18:00"}, {Code: "4103", Arr: "23:10:00", Dep: "23:58:05"}, {Code: "4101", Arr: "23:59:00"}}},
		// 출발 < 도착인 정차(신논현 24:02:10 → 24:00:00)도 역행: 제외.
		{Line: "9", Day: "SAT", Code: "C9201", Dir: "DOWN", Dest: "신논현", Stops: []seoulmetro.StopTime{
			{Code: "4101", Dep: "23:31:00"}, {Code: "4103", Arr: "24:02:10", Dep: "24:00:00"}, {Code: "4101", Arr: "24:05:00"}}},
	}}
	stopIDs := map[string]bool{"RS_ACC1_S-1-2731": true, "RS_ACC1_S-1-2732": true, "RS_ACC1_S-1-2733": true,
		"RS_ACC1_S-1-4101": true, "RS_ACC1_S-1-4103": true, "RS_ACC1_S-1-0222": true, "RS_ACC1_S-1-0260": true}
	routes, trips, stopTimes, st := metroRows(tt, stopIDs, map[string]string{"까치산(2호선)": "RS_ACC1_S-1-0260"})
	if st.Trips != 3 || st.SkippedStops != 2 || st.SkippedTrips != 1 || st.NameMatched != 1 || st.NonMonotonic != 2 {
		t.Fatalf("stats=%+v", st)
	}
	join := func(rows [][]string) string {
		var s []string
		for _, r := range rows {
			s = append(s, strings.Join(r, ","))
		}
		return strings.Join(s, "\n")
	}
	if join(routes) != "M_7,A_SEOULMETRO,7호선,7호선,1\nM_9_X,A_SEOULMETRO,9호선,9호선(급행),1\nM_2,A_SEOULMETRO,2호선,2호선,1" {
		t.Fatalf("routes=\n%s", join(routes))
	}
	if join(trips) != "M_7,WEEKDAY,M_7_DAY_7006,온수,1\nM_9_X,SUN,M_9_END_9502,중앙보훈병원,0\nM_2,WEEKDAY,M_2_DAY_2300,까치산,1" {
		t.Fatalf("trips=\n%s", join(trips))
	}
	want := "M_7_DAY_7006,05:39:00,05:39:00,RS_ACC1_S-1-2731,1\n" +
		"M_7_DAY_7006,05:41:30,05:41:50,RS_ACC1_S-1-2732,2\n" +
		"M_7_DAY_7006,05:43:20,05:43:20,RS_ACC1_S-1-2733,3\n" +
		"M_9_END_9502,23:58:00,23:58:00,RS_ACC1_S-1-4101,1\n" +
		"M_9_END_9502,24:03:00,24:03:00,RS_ACC1_S-1-4103,2\n" +
		"M_2_DAY_2300,07:00:00,07:00:30,RS_ACC1_S-1-0222,1\n" +
		"M_2_DAY_2300,07:05:00,07:05:00,RS_ACC1_S-1-0260,2"
	if join(stopTimes) != want {
		t.Fatalf("stop_times=\n%s\nwant\n%s", join(stopTimes), want)
	}
}

func pilotST(trip, at, stop, seq string) ktdb.Row {
	return ktdb.Row{"trip_id": trip, "arrival_time": at, "departure_time": at, "stop_id": stop, "stop_sequence": seq}
}

// Build 는 시각표가 있으면 파일럿 1~9호선 trip 을 버리고 시각표 trip 을 쓰며, 나머지 노선 trip 과 역은 그대로 둔다.
func TestBuildReplacesPilotMetroTrips(t *testing.T) {
	sub := &ktdb.Subway{
		Routes: []ktdb.Row{{"route_id": "RR_ACC1_S-1-07-1D", "route_short_name": "7호선", "route_long_name": "7호선<하행>"},
			{"route_id": "RR_ACC1_S-1-SB-1D", "route_short_name": "신분당선", "route_long_name": "신분당선<하행>"}},
		Trips: []ktdb.Row{{"route_id": "RR_ACC1_S-1-07-1D", "trip_id": "P7"},
			{"route_id": "RR_ACC1_S-1-SB-1D", "trip_id": "PSB"}},
		StopTimes: []ktdb.Row{
			pilotST("P7", "17:04:00", "RS_ACC1_S-1-2731", "1"), pilotST("P7", "17:06:00", "RS_ACC1_S-1-2732", "2"),
			pilotST("PSB", "06:00:00", "RS_ACC1_S-1-4307", "1"), pilotST("PSB", "06:03:00", "RS_ACC1_S-1-4308", "2"),
		},
		Stops: []ktdb.Row{
			{"stop_id": "RS_ACC1_S-1-2731", "stop_name": "청담(7호선)", "stop_lat": "37.5193", "stop_lon": "127.0532"},
			{"stop_id": "RS_ACC1_S-1-2732", "stop_name": "강남구청(7호선)", "stop_lat": "37.5172", "stop_lon": "127.0412"},
			{"stop_id": "RS_ACC1_S-1-4307", "stop_name": "강남(신분당선)", "stop_lat": "37.4966", "stop_lon": "127.0283"},
			{"stop_id": "RS_ACC1_S-1-4308", "stop_name": "양재(신분당선)", "stop_lat": "37.4843", "stop_lon": "127.0344"},
		},
	}
	metro := &seoulmetro.Timetable{Trains: []seoulmetro.Train{
		{Line: "7", Day: "DAY", Code: "7006", Dir: "DOWN", Dest: "온수", Stops: []seoulmetro.StopTime{
			{Code: "2731", Dep: "05:39:00"}, {Code: "2732", Arr: "05:41:30", Dep: "05:41:50"}}},
	}}
	out := filepath.Join(t.TempDir(), "g.zip")
	rep, err := Build(out, []BusRoute{sampleRoute()}, sub, nil, metro)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NMetroTrips != 1 || rep.NPilotTripsReplaced != 1 {
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
	if strings.Contains(got, "P7") || !strings.Contains(got, "PSB") || !strings.Contains(got, "M_7_DAY_7006") {
		t.Fatalf("파일럿 7호선 trip 은 빠지고 신분당선·시각표 trip 은 남아야: %v", tripIDs)
	}
	routes := readTable(t, zr, "routes.txt")
	var ids []string
	for _, r := range routes[1:] {
		ids = append(ids, r[0])
	}
	if strings.Contains(strings.Join(ids, ","), "RR_ACC1_S-1-07") || !strings.Contains(strings.Join(ids, ","), "M_7") {
		t.Fatalf("routes=%v", ids)
	}
	cal := readTable(t, zr, "calendar.txt")
	if len(cal) != 5 || cal[2][0] != "WEEKDAY" || cal[2][6] != "0" || cal[4][0] != "SUN" || cal[4][7] != "1" {
		t.Fatalf("calendar=%v", cal)
	}
	if !strings.Contains(strings.Join(services, ","), "WEEKDAY") {
		t.Fatalf("services=%v", services)
	}
	ag := readTable(t, zr, "agency.txt")
	if len(ag) != 4 || ag[3][0] != MetroAgencyID {
		t.Fatalf("agency=%v", ag)
	}
	st := readTable(t, zr, "stop_times.txt")
	var metroRows int
	for _, r := range st[1:] {
		if strings.HasPrefix(r[0], "M_7_") {
			metroRows++
		}
	}
	if metroRows != 2 {
		t.Fatalf("시각표 stop_times %d", metroRows)
	}
}
