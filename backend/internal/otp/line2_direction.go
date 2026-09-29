package otp

import "strings"

// line2Loop 은 2호선 본선 43역을 내선순환(시계 방향) 순서로 둔 것이다. 이름은 괄호 앞("종합운동장(잠실)" → "종합운동장").
// 출처: 생성 GTFS 의 M_2 본선 trip 정차 순서(2026-09-29). 서울 실시간 도착 API 의 updnLine 으로 내선이 시청→을지로입구,
// 왕십리→한양대 방향임을 확인했다(2026-09-29).
var line2Loop = []string{
	"시청", "을지로입구", "을지로3가", "을지로4가", "동대문역사문화공원", "신당", "상왕십리", "왕십리", "한양대", "뚝섬",
	"성수", "건대입구", "구의", "강변", "잠실나루", "잠실", "잠실새내", "종합운동장", "삼성", "선릉", "역삼", "강남", "교대",
	"서초", "방배", "사당", "낙성대", "서울대입구", "봉천", "신림", "신대방", "구로디지털단지", "대림", "신도림", "문래",
	"영등포구청", "당산", "합정", "홍대입구", "신촌", "이대", "아현", "충정로",
}

// line2Majors 는 서울교통공사 2호선 행선지 안내 개편안(2019-06 보도)의 주요 6역이다. 출발역 기준 진행 방향으로 가까운
// 2개 역을 방면으로 안내한다(예: 왕십리→충정로 방향은 "시청, 홍대입구 방면").
var line2Majors = map[string]bool{"시청": true, "왕십리": true, "잠실": true, "강남": true, "신도림": true, "홍대입구": true}

// line2Headsign 은 2호선 본선 구간의 행선지 앞에 순환 방향을 붙인다. 행선지가 "성수"(순환 운행)면 행선지 대신 진행
// 방향으로 가까운 주요역 2개("외선순환 시청·홍대입구" — 탑승역 자체는 세지 않는다), 그 밖의 종착역(단축·막차 열차)이면
// 그 종착역("내선순환 서울대입구")이다. 2호선이 아니거나 탑승역·다음 정차역이 본선에서 이웃이 아니면(성수·신정지선)
// 원래 행선지를 돌려준다.
func line2Headsign(route, from, next, headsign string) string {
	if route != "2호선" {
		return headsign
	}
	i, j := line2Index(from), line2Index(next)
	if i < 0 || j < 0 {
		return headsign
	}
	n := len(line2Loop)
	var step int
	var dir string
	switch j {
	case (i + 1) % n:
		step, dir = 1, "내선순환"
	case (i - 1 + n) % n:
		step, dir = n-1, "외선순환"
	default:
		return headsign
	}
	if headsign != "성수" {
		return dir + " " + headsign
	}
	var majors []string
	for k := 1; k < n && len(majors) < 2; k++ {
		if s := line2Loop[(i+k*step)%n]; line2Majors[s] {
			majors = append(majors, s)
		}
	}
	return dir + " " + strings.Join(majors, "·")
}

// line2Index 는 역 이름("왕십리(2호선)")의 본선 순서다. 본선 역이 아니면 -1.
func line2Index(name string) int {
	if i := strings.Index(name, "("); i >= 0 {
		name = name[:i]
	}
	name = strings.TrimSpace(name)
	for k, s := range line2Loop {
		if s == name {
			return k
		}
	}
	return -1
}
