package otp

import (
	"encoding/json"
	"strings"
	"testing"
)

// 안내(내비게이션)용 leg 상세: 도보 steps(방향·거리·도로명·역 출입구), 대중교통 중간 정차(이름·좌표·leg 출발 기준 초),
// 행선지(headsign). 쿼리가 이 필드들을 실제로 요청해야 값이 채워진다.
func TestLegStepsStopsHeadsign(t *testing.T) {
	for _, want := range []string{
		"headsign",
		"intermediatePlaces { name lat lon stop { gtfsId } arrival { scheduledTime } }",
		"absoluteDirection streetName bogusName distance lat lon exit",
		"... on Entrance { name }",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("쿼리에 %q 가 없다:\n%s", want, query)
		}
	}
	raw := `{"start":"2026-09-18T09:00:00+09:00","end":"2026-09-18T09:30:00+09:00","legs":[
	 {"mode":"WALK","start":{"scheduledTime":"2026-09-18T09:00:00+09:00"},
	  "end":{"scheduledTime":"2026-09-18T09:05:00+09:00"},
	  "from":{"name":"Origin"},"to":{"name":"강남(2호선)"},
	  "steps":[{"relativeDirection":"DEPART","absoluteDirection":"EAST","streetName":"테헤란로",
	            "distance":120.4,"lat":37.5,"lon":127.0},
	           {"relativeDirection":"RIGHT","absoluteDirection":"SOUTH","streetName":"sidewalk","bogusName":true,
	            "distance":50.2,"lat":37.501,"lon":127.0},
	           {"relativeDirection":"ENTER_STATION","absoluteDirection":"SOUTHEAST","streetName":"강남 8번 출구",
	            "distance":0,"lat":37.5011,"lon":127.0006,"feature":{"name":"강남 8번 출구"}}]},
	 {"mode":"SUBWAY","transitLeg":true,"headsign":"성수",
	  "start":{"scheduledTime":"2026-09-18T09:06:00+09:00"},
	  "end":{"scheduledTime":"2026-09-18T09:20:00+09:00"},
	  "from":{"name":"강남(2호선)"},"to":{"name":"성수"},
	  "intermediatePlaces":[{"name":"역삼","lat":37.5,"lon":127.03,"stop":{"gtfsId":"seoul:RS_1"},
	                         "arrival":{"scheduledTime":"2026-09-18T09:08:00+09:00"}},
	                        {"name":"선릉","lat":37.504,"lon":127.048,"stop":{"gtfsId":"seoul:RS_2"},
	                         "arrival":{"scheduledTime":"2026-09-18T09:10:00+09:00"}}]},
	 {"mode":"BUS","transitLeg":true,"headsign":"서울역",
	  "start":{"scheduledTime":"2026-09-18T09:21:00+09:00"},
	  "end":{"scheduledTime":"2026-09-18T09:29:00+09:00"},
	  "from":{"name":"정류장A"},"to":{"name":"정류장B"}}
	]}`
	var n node
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	it := n.itinerary()

	walk := it.Legs[0]
	if len(walk.Steps) != 3 {
		t.Fatalf("도보 steps=%d, want 3", len(walk.Steps))
	}
	if walk.Steps[0].Dir != "DEPART" || walk.Steps[0].Street != "테헤란로" || walk.Steps[0].Distance != 120.4 ||
		walk.Steps[0].Lat != 37.5 || walk.Steps[0].Abs != "EAST" {
		t.Errorf("steps[0]=%+v", walk.Steps[0])
	}
	if walk.Steps[1].Street != "" { // bogusName 인 생성 이름은 버린다
		t.Errorf("steps[1].Street=%q, want 빈 값", walk.Steps[1].Street)
	}
	if walk.Steps[2].Entrance != "강남 8번 출구" {
		t.Errorf("steps[2].Entrance=%q", walk.Steps[2].Entrance)
	}
	if len(walk.Stops) != 0 || walk.Headsign != "" {
		t.Errorf("도보 leg 에 정차·행선지가 붙었다: stops=%d headsign=%q", len(walk.Stops), walk.Headsign)
	}

	sub := it.Legs[1]
	if sub.Headsign != "성수" {
		t.Errorf("headsign=%q", sub.Headsign)
	}
	if len(sub.Stops) != 2 {
		t.Fatalf("정차=%d, want 2", len(sub.Stops))
	}
	// OffsetSec 은 leg 출발(09:06) 기준. 09:08 → 120, 09:10 → 240.
	if sub.Stops[0].Name != "역삼" || sub.Stops[0].OffsetSec != 120 || sub.Stops[0].StopID != "seoul:RS_1" {
		t.Errorf("stops[0]=%+v", sub.Stops[0])
	}
	if sub.Stops[1].OffsetSec != 240 || sub.Stops[1].Lat != 37.504 {
		t.Errorf("stops[1]=%+v", sub.Stops[1])
	}
	if sub.NextStop != "역삼" { // 방면 판별은 첫 중간 정차 이름 그대로
		t.Errorf("next_stop=%q, want 역삼", sub.NextStop)
	}

	if bus := it.Legs[2]; bus.NextStop != "정류장B" || len(bus.Stops) != 0 {
		t.Errorf("중간 정차 없는 대중교통 leg: next_stop=%q stops=%d", bus.NextStop, len(bus.Stops))
	}
}
