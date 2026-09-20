package build

import (
	"strings"

	"github.com/SIDED00R/seoul-route/gtfs/internal/seoulmetro"
)

// 서울교통공사 열차운행시각표(seoulmetro)로 1~9호선 trip 을 만드는 규칙. 수치 근거는 docs/gtfs-generator.md.
//   - 시각표 파일(공공데이터포털 15098251, 요일별 DAY/SAT/END)로 1~9호선 trip 을 전부 대체한다.
//   - 파일럿 trip 중 route_id 가 RR_ACC1_S-1-01-… ~ 09-… 인 것은 버린다(나머지 노선은 파일럿 그대로, service ALL).
//     역·부모역·transfers 는 파일럿 것을 계속 쓴다.
//   - route: 호선당 하나(M_<호선>), 급행은 M_<호선>_X. service: DAY→WEEKDAY(월~금), SAT→SAT, END→SUN(일). 공휴일은
//     calendar_dates 없이 요일 그대로 본다(알려진 한계). direction_id: UP·IN 0, DOWN·OUT 1. headsign 은 도착역.
//   - 시각표 역 코드가 파일럿 stops 에 없으면 "<역사명>(<호선>호선)" 이름으로 찾고, 그래도 없으면 그 정차만 뺀다
//     (SkippedStops). 남는 정차가 2개 미만이면 trip 을 뺀다.
//   - 시각이 역행하는 열차(도착이 앞 정차 출발보다 이르거나 출발이 도착보다 이른 정차가 있는 열차)는 통째로 뺀다
//     (NonMonotonic).
const (
	MetroAgencyID = "A_SEOULMETRO"
	metroPilotPfx = "RR_ACC1_S-1-0" // 파일럿 1~9호선 route_id 접두("RR_ACC1_S-1-01-1D" …)
)

type metroStats struct {
	Trips        int
	SkippedStops int // 파일럿에 없는 역 코드·이름이라 뺀 정차
	SkippedTrips int // 정차 2개 미만이라 뺀 열차
	NameMatched  int // 역 코드 대신 이름으로 찾은 정차
	NonMonotonic int // 시각이 역행해 뺀 열차
}

// metroReplacesRoute 는 파일럿 route_id 가 시각표로 대체되는 1~9호선인지.
func metroReplacesRoute(routeID string) bool {
	if !strings.HasPrefix(routeID, metroPilotPfx) {
		return false
	}
	rest := routeID[len(metroPilotPfx):]
	return len(rest) >= 2 && rest[0] >= '1' && rest[0] <= '9' && rest[1] == '-'
}

// metroRows 는 routes·trips·stop_times 행을 만든다. stopIDs 는 파일럿 stop_id 집합, stopByName 은 파일럿 stop_name → stop_id.
// 열차는 입력 순(호선·요일·코드)이라 결정적이다.
func metroRows(tt *seoulmetro.Timetable, stopIDs map[string]bool, stopByName map[string]string) (
	routes, trips, stopTimes [][]string, st metroStats) {
	seenRoute := map[string]bool{}
	for _, t := range tt.Trains {
		routeID := "M_" + t.Line
		name := t.Line + "호선"
		if t.Express {
			routeID += "_X"
			name += "(급행)"
		}
		var rows [][]string
		prevDep := ""
		monotonic := true
		for _, s := range t.Stops {
			id := "RS_ACC1_S-1-" + s.Code
			if !stopIDs[id] {
				byName, ok := stopByName[s.Name+"("+t.Line+"호선)"]
				if !ok {
					st.SkippedStops++
					continue
				}
				id = byName
				st.NameMatched++
			}
			arr, dep := s.Arr, s.Dep
			if arr == "" {
				arr = dep
			}
			if dep == "" {
				dep = arr
			}
			if arr < prevDep || dep < arr {
				monotonic = false
				break
			}
			prevDep = dep
			rows = append(rows, []string{"", arr, dep, id, ""})
		}
		if !monotonic {
			st.NonMonotonic++
			continue
		}
		if len(rows) < 2 {
			st.SkippedTrips++
			continue
		}
		if !seenRoute[routeID] {
			seenRoute[routeID] = true
			c := metroColor(t.Line)
			routes = append(routes, []string{routeID, MetroAgencyID, t.Line + "호선", name, "1", c, textColor(c)})
		}
		tripID := "M_" + t.Line + "_" + t.Day + "_" + t.Code
		dir := "0"
		if t.Dir == "DOWN" || t.Dir == "OUT" {
			dir = "1"
		}
		trips = append(trips, []string{routeID, metroService(t.Day), tripID, t.Dest, dir})
		for i, r := range rows {
			r[0] = tripID
			r[4] = itoa(i + 1)
			stopTimes = append(stopTimes, r)
		}
		st.Trips++
	}
	return routes, trips, stopTimes, st
}

func metroService(day string) string {
	switch day {
	case "SAT":
		return "SAT"
	case "END":
		return "SUN"
	default:
		return "WEEKDAY"
	}
}

// metroCalendar 는 시각표용 service 행(월~금 / 토 / 일).
func metroCalendar() [][]string {
	return [][]string{
		{"WEEKDAY", "1", "1", "1", "1", "1", "0", "0", ServiceStart, ServiceEnd},
		{"SAT", "0", "0", "0", "0", "0", "1", "0", ServiceStart, ServiceEnd},
		{"SUN", "0", "0", "0", "0", "0", "0", "1", ServiceStart, ServiceEnd},
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
