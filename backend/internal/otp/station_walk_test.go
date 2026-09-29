package otp

import (
	"encoding/json"
	"testing"

	"github.com/SIDED00R/seoul-route/backend/internal/crossing"
)

// 역으로 들어가는 도보: 출입구(ENTER_STATION)에서 끝나고, 역 안 통로(출입구→승강장)는 경로선·steps 에서 빠진다.
// 역에서 나오는 도보: 출입구(EXIT_STATION)에서 시작한다. 둘 다 지나는 환승 도보는 양끝을 자른다. 거리·시각은 그대로다.
func TestTrimStationWalk(t *testing.T) {
	p := func(lat, lon float64) crossing.Point { return crossing.Point{Lat: lat, Lon: lon} }
	start, turn, gate, platform := p(37.5630, 126.9740), p(37.5640, 126.9755), p(37.5650, 126.9765), p(37.5657, 126.9769)
	access := Leg{Mode: "WALK", FromName: "Origin", ToName: "시청", ToLat: platform.Lat, ToLon: platform.Lon,
		Start: "2026-01-05T08:00:00+09:00", End: "2026-01-05T08:08:00+09:00", Duration: 480, Distance: 500,
		Polyline: encodePolyline([]crossing.Point{start, turn, gate, platform}),
		Steps: []Step{
			{Dir: "DEPART", Distance: 170, Lat: start.Lat, Lon: start.Lon},
			{Dir: "LEFT", Distance: 226, Lat: turn.Lat, Lon: turn.Lon},
			{Dir: "ENTER_STATION", Lat: gate.Lat, Lon: gate.Lon, Entrance: "시청 4번 출구", Street: "시청 4번 출구"},
			{Dir: "LEFT", Distance: 104, Lat: gate.Lat, Lon: gate.Lon},
		}}
	trimStationWalk(&access)
	pts := crossing.DecodePolyline(access.Polyline)
	if access.ToName != "시청 4번 출구" || access.ToLat != gate.Lat || access.ToLon != gate.Lon ||
		len(pts) != 3 || pts[2] != gate || len(access.Steps) != 3 || access.Steps[2].Dir != "ENTER_STATION" ||
		access.Distance != 500 || access.End != "2026-01-05T08:08:00+09:00" || access.Duration != 480 {
		t.Errorf("들어가는 도보: %+v pts=%v", access, pts)
	}

	exitGate, street, dest := p(37.5663, 126.9832), p(37.5665, 126.9850), p(37.5670, 126.9900)
	egress := Leg{Mode: "WALK", FromName: "을지로입구", FromLat: 37.5660, FromLon: 126.9826, ToName: "Destination",
		Distance: 704, Polyline: encodePolyline([]crossing.Point{p(37.5660, 126.9826), exitGate, street, dest}),
		Steps: []Step{
			{Dir: "DEPART", Distance: 44, Lat: 37.5660, Lon: 126.9826},
			{Dir: "EXIT_STATION", Lat: exitGate.Lat, Lon: exitGate.Lon, Entrance: "을지로입구 4번 출구"},
			{Dir: "LEFT", Distance: 200, Lat: exitGate.Lat, Lon: exitGate.Lon},
			{Dir: "RIGHT", Distance: 460, Lat: street.Lat, Lon: street.Lon},
		}}
	trimStationWalk(&egress)
	pts = crossing.DecodePolyline(egress.Polyline)
	if egress.FromName != "을지로입구 4번 출구" || egress.FromLat != exitGate.Lat || len(pts) != 3 || pts[0] != exitGate ||
		egress.Steps[0].Dir != "EXIT_STATION" || egress.Distance != 704 || egress.ToName != "Destination" {
		t.Errorf("나오는 도보: %+v pts=%v", egress, pts)
	}

	transfer := Leg{Mode: "WALK", FromName: "A", ToName: "B", Distance: 320,
		Polyline: encodePolyline([]crossing.Point{p(37.50, 127.00), p(37.501, 127.001), p(37.503, 127.003),
			p(37.504, 127.004)}),
		Steps: []Step{
			{Dir: "DEPART", Distance: 30, Lat: 37.50, Lon: 127.00},
			{Dir: "EXIT_STATION", Lat: 37.501, Lon: 127.001, Entrance: "A 1번 출구"},
			{Dir: "LEFT", Distance: 250, Lat: 37.501, Lon: 127.001},
			{Dir: "ENTER_STATION", Lat: 37.503, Lon: 127.003, Entrance: "B 2번 출구"},
			{Dir: "RIGHT", Distance: 40, Lat: 37.503, Lon: 127.003},
		}}
	trimStationWalk(&transfer)
	pts = crossing.DecodePolyline(transfer.Polyline)
	if transfer.FromName != "A 1번 출구" || transfer.ToName != "B 2번 출구" || len(pts) != 2 || len(transfer.Steps) != 3 ||
		transfer.Distance != 320 {
		t.Errorf("환승 도보: %+v pts=%v", transfer, pts)
	}

	// 역을 지나가기만 하는 도보(대합실 통로로 큰길 건넘): 들어갔다 나오므로 자르지 않는다.
	through := Leg{Mode: "WALK", ToName: "D", Distance: 400,
		Polyline: encodePolyline([]crossing.Point{p(37.50, 127.00), p(37.501, 127.001), p(37.502, 127.002),
			p(37.503, 127.003)}),
		Steps: []Step{
			{Dir: "DEPART", Distance: 100, Lat: 37.50, Lon: 127.00},
			{Dir: "ENTER_STATION", Lat: 37.501, Lon: 127.001, Entrance: "E 1번 출구"},
			{Dir: "LEFT", Distance: 150, Lat: 37.501, Lon: 127.001},
			{Dir: "EXIT_STATION", Lat: 37.502, Lon: 127.002, Entrance: "E 5번 출구"},
			{Dir: "RIGHT", Distance: 150, Lat: 37.502, Lon: 127.002},
		}}
	throughPoly := through.Polyline
	trimStationWalk(&through)
	if through.ToName != "D" || through.Polyline != throughPoly || len(through.Steps) != 5 || through.Distance != 400 {
		t.Errorf("지나가는 도보가 잘렸다: %+v", through)
	}

	plain := Leg{Mode: "WALK", ToName: "C", Distance: 300, Polyline: encodePolyline([]crossing.Point{start, turn}),
		Steps: []Step{{Dir: "DEPART", Distance: 300, Lat: start.Lat, Lon: start.Lon}}}
	before := plain.Polyline
	trimStationWalk(&plain)
	if plain.ToName != "C" || plain.Distance != 300 || plain.Polyline != before || len(plain.Steps) != 1 {
		t.Errorf("출입구 없는 도보가 바뀌었다: %+v", plain)
	}
}

// 배선: OTP 응답의 도보 leg 가 itinerary() 에서 출입구로 잘린다.
func TestItineraryTrimsStationWalk(t *testing.T) {
	poly := encodePolyline([]crossing.Point{{Lat: 37.5640, Lon: 126.9755}, {Lat: 37.5650, Lon: 126.9765},
		{Lat: 37.5657, Lon: 126.9769}})
	raw := `{"legs":[{"mode":"WALK","distance":330,"from":{"name":"Origin"},
	  "to":{"name":"시청","lat":37.5657,"lon":126.9769},
	  "legGeometry":{"points":` + jsonStr(poly) + `},
	  "steps":[{"relativeDirection":"DEPART","distance":226,"lat":37.5640,"lon":126.9755},
	           {"relativeDirection":"ENTER_STATION","distance":0,"lat":37.5650,"lon":126.9765,"streetName":"시청 4번 출구",
	            "feature":{"name":"시청 4번 출구"}},
	           {"relativeDirection":"LEFT","distance":104,"lat":37.5650,"lon":126.9765}]}]}`
	var n node
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	w := n.itinerary().Legs[0]
	if w.ToName != "시청 4번 출구" || w.ToLat != 37.5650 || len(w.Steps) != 2 || w.Distance != 330 {
		t.Errorf("walk=%+v", w)
	}
}
