package build

import "strings"

// 노선색 표 (2026-09-18 기준, GTFS route_color 는 # 없는 6자리 16진수)
//   - 지하철: ko.wikipedia 「틀:한국 철도 노선색」. 코레일 철도 사인류 설치 기준 매뉴얼·서울시 공공시설물 표준형디자인에서
//     온 값들이라 실제 안내 표지와 같다. 노선이 새로 생기거나 색이 바뀌면 이 표를 고친다.
//   - 버스: 리브레위키 「틀:버스 노선색」. 서울시는 색 체계(간선 파랑·지선 초록·광역 빨강·순환 노랑)를 문서로 밝히지만
//     16진수 값은 공표하지 않아 통용값을 쓴다. 서울시가 2024~25 년 색 체계 개편을 검토 중이라 바뀌면 갱신한다.
//   - 글자색: 배경이 밝은 노선(9호선·경의중앙·수인분당·우이신설·서해·의정부·인천1)과 순환버스는 검정,
//     심야·동행버스(3D5BAB)는 노랑(FFC600), 나머지는 흰색.

const (
	colorWhite = "FFFFFF"
	colorBlack = "000000"
)

// metroColors 는 서울교통공사 1~9호선(seoulmetro 의 Line 값) 색.
var metroColors = map[string]string{
	"1": "0052A4", "2": "00A84D", "3": "EF7C1C", "4": "00A5DE", "5": "996CAC",
	"6": "CD7C2F", "7": "747F00", "8": "E6186C", "9": "BDB092",
}

// kricColors 는 레일포털 노선 코드(kricLineOf)별 색. 코드는 파일럿 route_id 에서 온다(생성 GTFS 의 K_ 접미사와 같다).
// 운영기관 코드(agency_id 의 A_UI 등)와 헷갈리기 쉽다 — 우이신설은 노선 WS·기관 UI, 의정부는 노선 UI·기관 UL 이다.
var kricColors = map[string]string{
	"KJ": "77C4A3", // 경의중앙
	"SD": "F5A200", // 수인분당
	"GC": "0C8E72", // 경춘
	"KK": "003DA5", // 경강
	"AP": "0090D2", // 공항철도
	"SB": "D4003B", // 신분당
	"UI": "FDA600", // 의정부경전철
	"SL": "6789CA", // 신림
	"WS": "B0CE18", // 우이신설
	"KP": "A17800", // 김포도시철도
	"I1": "7CA8D5", // 인천1
	"I2": "ED8B00", // 인천2
}

// pilotSubwayColors 는 노선 코드 표에 없는 파일럿 노선(서해선·GTX-A)의 색. 노선 이름으로 찾는다.
var pilotSubwayColors = map[string]string{"서해": "81A914", "GTX-A": "9A6292"}

// pilotColor 는 파일럿 도시철도 노선의 색. 시각표 CSV 가 없으면 1~9호선·레일포털 12개 노선도 파일럿 행으로 나오므로
// route_id 의 노선 코드로 먼저 찾는다. 파일럿 코드는 0 을 채운 "01"~"09" 라 metroColors 키("1"~"9")와 다르다.
func pilotColor(routeID, name string) string {
	code := kricLineOf(routeID)
	if len(code) == 2 && code[0] == '0' {
		if c := metroColors[code[1:]]; c != "" {
			return c
		}
	}
	if c := kricColors[code]; c != "" {
		return c
	}
	return pilotSubwayColor(name)
}

// busColors 는 서울 버스 API routeType 별 색. 1 공항·7 인천·8 경기·10 관광·14 한강·0 공용은 비워 둔다
// (서울시 색 체계가 정한 유형이 아니다. 앱이 수단별 기본 팔레트를 쓴다).
var busColors = map[string]string{
	"2":  "53B332", // 마을(지선과 같은 초록)
	"3":  "0068B7", // 간선
	"4":  "53B332", // 지선
	"5":  "F2B70A", // 순환
	"6":  "E60012", // 광역
	"13": "3D5BAB", // 동행(심야)
	"15": "3D5BAB", // 심야
}

// lightBackground 는 흰 글자가 읽히지 않는 밝은 배경색이다.
var lightBackground = map[string]bool{
	"BDB092": true, "77C4A3": true, "F5A200": true, "B0CE18": true,
	"81A914": true, "FDA600": true, "7CA8D5": true, "F2B70A": true,
}

// textColor 는 배경색 위에 얹을 글자색.
func textColor(bg string) string {
	if bg == "" {
		return ""
	}
	if lightBackground[bg] {
		return colorBlack
	}
	if bg == "3D5BAB" { // 심야버스는 남색 바탕에 노란 글자(실제 차량 도색)
		return "FFC600"
	}
	return colorWhite
}

// metroColor 는 서울교통공사 호선 색. 급행도 같은 색이다.
func metroColor(line string) string { return metroColors[line] }

// kricColor 는 레일포털 노선 코드 색.
func kricColor(code string) string { return kricColors[code] }

// pilotSubwayColor 는 파일럿 노선 이름("서해선"·"GTX-A")으로 색을 찾는다.
func pilotSubwayColor(name string) string {
	for key, c := range pilotSubwayColors {
		if strings.Contains(name, key) {
			return c
		}
	}
	return ""
}

// busColor 는 서울 버스 routeType 색.
func busColor(routeType string) string { return busColors[routeType] }
