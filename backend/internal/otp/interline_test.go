package otp

import (
	"encoding/json"
	"testing"

	"github.com/SIDED00R/seoul-route/backend/internal/crossing"
)

func TestEncodePolylineRoundTrip(t *testing.T) {
	pts := []crossing.Point{{Lat: 37.56123, Lon: 127.03712}, {Lat: 37.55501, Lon: 127.04402},
		{Lat: 37.54461, Lon: 127.056}}
	got := crossing.DecodePolyline(encodePolyline(pts))
	if len(got) != len(pts) {
		t.Fatalf("got %v", got)
	}
	for i := range pts {
		if got[i] != pts[i] {
			t.Errorf("%d: got %v want %v", i, got[i], pts[i])
		}
	}
}

// 배선: 같은 열차를 이어 탄 구간(interlineWithPreviousLeg)은 앞 구간에 합쳐 한 구간이 된다. 이어 탄 역(성수)은 중간 정차가
// 되고 뒤 정차의 초는 앞 구간 출발 기준으로 옮기며, 경로선은 이어 붙인다. 이어 타지 않은 구간은 그대로 둔다.
func TestItineraryJoinsInterlinedLegs(t *testing.T) {
	p1 := encodePolyline([]crossing.Point{{Lat: 37.5612, Lon: 127.0371}, {Lat: 37.5474, Lon: 127.0474},
		{Lat: 37.5446, Lon: 127.056}})
	p2 := encodePolyline([]crossing.Point{{Lat: 37.5446, Lon: 127.056}, {Lat: 37.5404, Lon: 127.0692}})
	raw := `{"start":"2026-10-06T07:55:00+09:00","end":"2026-10-06T08:10:00+09:00","numberOfTransfers":0,"legs":[
	 {"mode":"WALK","start":{"scheduledTime":"2026-10-06T07:55:00+09:00"},
	  "end":{"scheduledTime":"2026-10-06T07:56:00+09:00"},"from":{"name":"Origin"},"to":{"name":"왕십리(2호선)"}},
	 {"mode":"SUBWAY","transitLeg":true,"headsign":"성수","route":{"shortName":"2호선","gtfsId":"seoul:M_2"},
	  "duration":270,"distance":2500,
	  "start":{"scheduledTime":"2026-10-06T07:56:00+09:00"},"end":{"scheduledTime":"2026-10-06T08:00:30+09:00"},
	  "from":{"name":"왕십리(2호선)","stop":{"gtfsId":"seoul:RS_W"}},"to":{"name":"성수","lat":37.5446,"lon":127.056,
	  "stop":{"gtfsId":"seoul:RS_SS"}},"legGeometry":{"points":` + jsonStr(p1) + `},
	  "intermediatePlaces":[{"name":"한양대","arrival":{"scheduledTime":"2026-10-06T07:57:30+09:00"}},
	                        {"name":"뚝섬","arrival":{"scheduledTime":"2026-10-06T07:59:00+09:00"}}]},
	 {"mode":"SUBWAY","transitLeg":true,"interlineWithPreviousLeg":true,"headsign":"성수",
	  "route":{"shortName":"2호선","gtfsId":"seoul:M_2"},"duration":180,"distance":1400,
	  "start":{"scheduledTime":"2026-10-06T08:01:30+09:00"},"end":{"scheduledTime":"2026-10-06T08:04:30+09:00"},
	  "from":{"name":"성수","stop":{"gtfsId":"seoul:RS_SS"}},"to":{"name":"구의(광진구청)"},
	  "intermediatePlaces":[{"name":"건대입구(2호선)","arrival":{"scheduledTime":"2026-10-06T08:03:00+09:00"}}],
	  "legGeometry":{"points":` + jsonStr(p2) + `}},
	 {"mode":"WALK","start":{"scheduledTime":"2026-10-06T08:04:30+09:00"},
	  "end":{"scheduledTime":"2026-10-06T08:10:00+09:00"},"from":{"name":"구의(광진구청)"},"to":{"name":"Destination"}}]}`
	var n node
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	it := n.itinerary()
	if len(it.Legs) != 3 {
		t.Fatalf("legs=%d, want 3(도보·2호선·도보)", len(it.Legs))
	}
	sub := it.Legs[1]
	if sub.FromName != "왕십리(2호선)" || sub.ToName != "구의(광진구청)" || sub.End != "2026-10-06T08:04:30+09:00" ||
		sub.Duration != 510 || sub.Distance != 3900 || sub.FromStopID != "seoul:RS_W" || sub.NextStop != "한양대" ||
		sub.Headsign != "내선순환 잠실·강남" {
		t.Errorf("합친 구간: %+v", sub)
	}
	var names []string
	var offs []int
	for _, s := range sub.Stops {
		names = append(names, s.Name)
		offs = append(offs, s.OffsetSec)
	}
	// 출발 07:56 기준: 성수 도착 08:00:30 = 270초, 이어 탄 뒤 건대입구 08:03:00 = 420초
	if len(sub.Stops) != 4 || names[2] != "성수" || sub.Stops[2].StopID != "seoul:RS_SS" || offs[0] != 90 ||
		offs[1] != 180 || offs[2] != 270 || names[3] != "건대입구(2호선)" || offs[3] != 420 {
		t.Errorf("정차=%v %v", names, offs)
	}
	pts := crossing.DecodePolyline(sub.Polyline)
	if len(pts) != 4 || pts[3] != (crossing.Point{Lat: 37.5404, Lon: 127.0692}) {
		t.Errorf("경로선=%v", pts)
	}
	if it.Legs[2].FromName != "구의(광진구청)" || it.Legs[2].Mode != "WALK" {
		t.Errorf("뒤 도보=%+v", it.Legs[2])
	}
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
