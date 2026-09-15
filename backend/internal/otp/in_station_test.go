package otp

import (
	"encoding/json"
	"strings"
	"testing"
)

// 도보 leg 양끝이 같은 부모역의 정류장이고 steps 에 역 출입구(EXIT_STATION/ENTER_STATION)가 없으면 InStation(역 안 환승
// 통로). 부모역이 다르거나, 한쪽이 좌표(정류장 없음)거나, 대중교통 leg 거나, 같은 부모역이라도 출입구로 나갔다 들어오는
// 도보(신촌 2호선↔경의중앙선)면 false. 쿼리가 parentStation 과 steps 를 실제로 요청해야 이 값이 채워진다(이슈 #57).
func TestInStationWalkLeg(t *testing.T) {
	if strings.Count(query, "parentStation { gtfsId }") != 2 {
		t.Fatalf("쿼리가 from·to 의 parentStation 을 요청하지 않는다:\n%s", query)
	}
	if !strings.Contains(query, "steps { relativeDirection }") {
		t.Fatalf("쿼리가 steps 의 relativeDirection 을 요청하지 않는다:\n%s", query)
	}
	raw := `{"start":"s","end":"e","legs":[
	 {"mode":"WALK","from":{"name":"Origin"},"to":{"name":"사당(4호선)",
	   "stop":{"gtfsId":"seoul:RS_ACC1_S-1-0433","parentStation":{"gtfsId":"seoul:ST_사당"}}}},
	 {"mode":"SUBWAY","transitLeg":true,
	   "from":{"name":"사당(4호선)","stop":{"gtfsId":"seoul:RS_ACC1_S-1-0433","parentStation":{"gtfsId":"seoul:ST_사당"}}},
	   "to":{"name":"사당(4호선)","stop":{"gtfsId":"seoul:RS_ACC1_S-1-0433","parentStation":{"gtfsId":"seoul:ST_사당"}}}},
	 {"mode":"WALK","steps":[{"relativeDirection":"DEPART"}],
	   "from":{"name":"사당(4호선)","stop":{"gtfsId":"seoul:RS_ACC1_S-1-0433","parentStation":{"gtfsId":"seoul:ST_사당"}}},
	   "to":{"name":"사당(2호선)","stop":{"gtfsId":"seoul:RS_ACC1_S-1-0226","parentStation":{"gtfsId":"seoul:ST_사당"}}}},
	 {"mode":"WALK",
	   "from":{"name":"낙성대","stop":{"gtfsId":"seoul:RS_ACC1_S-1-0227","parentStation":{"gtfsId":"seoul:ST_낙성대"}}},
	   "to":{"name":"사당(2호선)","stop":{"gtfsId":"seoul:RS_ACC1_S-1-0226","parentStation":{"gtfsId":"seoul:ST_사당"}}}},
	 {"mode":"WALK",
	   "from":{"name":"정류장A","stop":{"gtfsId":"seoul:BS_1"}},"to":{"name":"정류장B","stop":{"gtfsId":"seoul:BS_2"}}},
	 {"mode":"WALK","steps":[{"relativeDirection":"DEPART"},{"relativeDirection":"EXIT_STATION"},
	   {"relativeDirection":"HARD_RIGHT"},{"relativeDirection":"RIGHT"},{"relativeDirection":"ENTER_STATION"},
	   {"relativeDirection":"LEFT"}],
	   "from":{"name":"신촌(2호선)","stop":{"gtfsId":"seoul:RS_ACC1_S-1-0240","parentStation":{"gtfsId":"seoul:ST_신촌"}}},
	   "to":{"name":"신촌(경의중앙선)","stop":{"gtfsId":"seoul:RS_ACC1_S-1-1252","parentStation":{"gtfsId":"seoul:ST_신촌"}}}},
	 {"mode":"WALK","steps":[{"relativeDirection":"DEPART"},{"relativeDirection":"ENTER_STATION"}],
	   "from":{"name":"신촌(경의중앙선)","stop":{"gtfsId":"seoul:RS_ACC1_S-1-1252","parentStation":{"gtfsId":"seoul:ST_신촌"}}},
	   "to":{"name":"신촌(2호선)","stop":{"gtfsId":"seoul:RS_ACC1_S-1-0240","parentStation":{"gtfsId":"seoul:ST_신촌"}}}},
	 {"mode":"WALK","steps":[{"relativeDirection":"DEPART"},{"relativeDirection":"EXIT_STATION"}],
	   "from":{"name":"신촌(2호선)","stop":{"gtfsId":"seoul:RS_ACC1_S-1-0240","parentStation":{"gtfsId":"seoul:ST_신촌"}}},
	   "to":{"name":"신촌(경의중앙선)","stop":{"gtfsId":"seoul:RS_ACC1_S-1-1252","parentStation":{"gtfsId":"seoul:ST_신촌"}}}}
	]}`
	var n node
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	want := []bool{false, false, true, false, false, false, false, false}
	it := n.itinerary()
	for i, w := range want {
		if it.Legs[i].InStation != w {
			t.Errorf("leg %d (%s %s→%s) InStation=%v, want %v", i, it.Legs[i].Mode, it.Legs[i].FromName,
				it.Legs[i].ToName, it.Legs[i].InStation, w)
		}
	}
}
