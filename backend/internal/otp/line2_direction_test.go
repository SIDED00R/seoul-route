package otp

import (
	"encoding/json"
	"testing"
)

func TestLine2Headsign(t *testing.T) {
	cases := []struct {
		route, from, next, headsign, want string
	}{
		// 개편안 보도 예시: 왕십리에서 충정로 쪽은 "시청, 홍대입구", 반대편은 "잠실, 강남"
		{"2호선", "왕십리(2호선)", "상왕십리", "성수", "외선순환 시청·홍대입구"},
		{"2호선", "왕십리(2호선)", "한양대", "성수", "내선순환 잠실·강남"},
		// 탑승역이 주요역이면 그다음 주요역부터
		{"2호선", "강남(2호선)", "역삼", "성수", "외선순환 잠실·왕십리"},
		// 순서표 끝을 넘어간다(충정로→시청, 시청→충정로)
		{"2호선", "충정로(2호선)", "시청(2호선)", "성수", "내선순환 시청·왕십리"},
		{"2호선", "시청(2호선)", "충정로", "성수", "외선순환 홍대입구·신도림"},
		// 괄호가 두 번 붙은 이름
		{"2호선", "종합운동장(잠실)(2호선)", "삼성", "성수", "내선순환 강남·신도림"},
		// 성수행이 아닌 본선 열차(단축·막차)는 방향 + 종착역
		{"2호선", "강남(2호선)", "교대(2호선)", "서울대입구", "내선순환 서울대입구"},
		{"2호선", "홍대입구(2호선)", "합정(2호선)", "신도림", "외선순환 신도림"},
		{"2호선", "사당(2호선)", "낙성대", "신도림", "내선순환 신도림"},
		// 지선·이웃이 아닌 역·다른 노선은 원래 행선지
		{"2호선", "성수(2호선)", "용답", "신설동", "신설동"},
		{"2호선", "신도림(2호선)", "도림천", "까치산", "까치산"},
		{"2호선", "시청(2호선)", "을지로3가", "성수", "성수"},
		{"인천2호선", "시청", "충정로", "운연", "운연"},
	}
	for _, c := range cases {
		if got := line2Headsign(c.route, c.from, c.next, c.headsign); got != c.want {
			t.Errorf("%s %s→%s: got %q want %q", c.route, c.from, c.next, got, c.want)
		}
	}
}

func TestLine2LoopWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range line2Loop {
		if seen[s] {
			t.Errorf("중복 %q", s)
		}
		seen[s] = true
	}
	if len(line2Loop) != 43 {
		t.Errorf("본선 %d역, want 43", len(line2Loop))
	}
	for s := range line2Majors {
		if !seen[s] {
			t.Errorf("주요역 %q 가 본선에 없다", s)
		}
	}
}

// 배선: OTP 응답의 2호선 leg 는 다음 정차(중간 정차가 없으면 하차역)로 방향을 정한다.
func TestItineraryLine2Headsign(t *testing.T) {
	raw := `{"legs":[
	 {"mode":"SUBWAY","transitLeg":true,"headsign":"성수","route":{"shortName":"2호선"},
	  "from":{"name":"왕십리(2호선)"},"to":{"name":"시청(2호선)"},
	  "intermediatePlaces":[{"name":"상왕십리"},{"name":"신당"}]},
	 {"mode":"SUBWAY","transitLeg":true,"headsign":"성수","route":{"shortName":"2호선"},
	  "from":{"name":"왕십리(2호선)"},"to":{"name":"한양대(2호선)"}}]}`
	var n node
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	it := n.itinerary()
	if it.Legs[0].Headsign != "외선순환 시청·홍대입구" || it.Legs[1].Headsign != "내선순환 잠실·강남" {
		t.Errorf("headsign=%q, %q", it.Legs[0].Headsign, it.Legs[1].Headsign)
	}
}
