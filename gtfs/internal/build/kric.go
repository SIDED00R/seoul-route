package build

import (
	"sort"
	"strings"

	"github.com/SIDED00R/seoul-route/gtfs/internal/kric"
)

// 레일포털(KRIC) 시각표(kric)로 코레일·민자 노선 trip 을 만드는 규칙. 1~9호선(metro.go)과 같은 구조다.
//   - 대상 노선은 kric-stations.csv 에 있는 것(otp/fetch_kric_timetable.py 의 TARGETS). 그 노선의 파일럿 route_id
//     `RR_ACC1_S-1-<코드>-…` 의 route·trip 을 버리고 시각표 trip 으로 대체한다. 역·부모역·통로는 파일럿 것을 계속 쓴다.
//   - 레일포털 역 코드는 파일럿 stop_id 와 무관하다. 그 노선의 파일럿 정차역 가운데 괄호 앞 이름이 같은 것(여럿이면
//     좌표가 가장 가까운 것)에 붙이고, 이름이 없으면 KricNearestM 안에서 가장 가까운 역에 붙인다(NearestMatched).
//     둘 다 없으면 그 정차만 뺀다(SkippedStops). 남는 정차가 2개 미만이면 열차를 뺀다(SkippedTrips).
//   - service: 8 평일→WEEKDAY, 7 토→SAT, 9 휴일→SUN. 토요일 시각표가 없는 노선(코레일·공항철도·신분당 등, 2026-09-14
//     실측)은 휴일 열차를 SATSUN(토·일) 으로 넣는다.
//   - direction_id: 첫 정차의 노선 내 순서 < 마지막 정차 순서면 0, 아니면 1. headsign 은 종점역 이름.
//   - 시각이 역행하는 열차는 통째로 뺀다(NonMonotonic, metro.go 와 같은 이유).
const KricNearestM = 300.0

type kricStats struct {
	Trips          int
	SkippedStops   int
	SkippedTrips   int
	NearestMatched int
	NonMonotonic   int
}

// pilotStop 은 파일럿 stop 한 개(이름·좌표). kricRows 가 노선별 후보로 쓴다.
type pilotStop struct {
	ID       string
	Name     string
	Lat, Lon float64
}

// kricLineOf 는 파일럿 route_id "RR_ACC1_S-1-KJ-1D" 에서 노선 코드 "KJ" 를 꺼낸다. 형식이 다르면 "".
func kricLineOf(routeID string) string {
	parts := strings.Split(routeID, "-")
	if len(parts) < 4 || parts[0] != "RR_ACC1_S" {
		return ""
	}
	return parts[2]
}

// kricAgencies 는 시각표에 있는 운영기관마다 agency 행을 만든다(코드 순).
func kricAgencies(tt *kric.Timetable) [][]string {
	names := map[string]string{"KR": "코레일", "AR": "공항철도", "DX": "신분당선(네오트랜스)", "UL": "의정부경전철",
		"SL": "신림선(남서울경전철)", "UI": "우이신설경전철", "GM": "김포골드라인", "IC": "인천교통공사"}
	seen := map[string]bool{}
	var rows [][]string
	for _, code := range sortedLineCodes(tt) {
		opr := tt.Lines[code].Opr
		if seen[opr] {
			continue
		}
		seen[opr] = true
		name := names[opr]
		if name == "" {
			name = opr
		}
		rows = append(rows, []string{"A_" + opr, name, "https://data.kric.go.kr/", "Asia/Seoul"})
	}
	return rows
}

func sortedLineCodes(tt *kric.Timetable) []string {
	codes := make([]string, 0, len(tt.Lines))
	for c := range tt.Lines {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return codes
}

// kricNeedsSatSun: 토요일 시각표가 없는 노선이 하나라도 있으면 SATSUN service 가 필요하다.
func kricNeedsSatSun(tt *kric.Timetable) bool {
	for _, l := range tt.Lines {
		if !l.HasSat {
			return true
		}
	}
	return false
}

// kricRows 는 routes·trips·stop_times 행을 만든다. pilotStops 는 노선 코드 → 그 노선 파일럿 trip 이 서는 stop 들,
// pilotNames 는 노선 코드 → 파일럿 route_short_name(앱 표시용, 예 "경의중앙선").
func kricRows(tt *kric.Timetable, pilotStops map[string][]pilotStop, pilotNames map[string]string) (
	routes, trips, stopTimes [][]string, st kricStats) {
	stopOf := map[string]map[string]string{}          // 노선 → 레일포털 역 코드 → 파일럿 stop_id
	stationOf := map[string]map[string]kric.Station{} // 노선 → 역 코드 → 역
	for _, code := range sortedLineCodes(tt) {
		l := tt.Lines[code]
		stopOf[code] = map[string]string{}
		stationOf[code] = map[string]kric.Station{}
		for _, s := range l.Stations {
			stationOf[code][s.Code] = s
			if id, nearest, ok := matchPilotStop(s, pilotStops[code]); ok {
				stopOf[code][s.Code] = id
				if nearest {
					st.NearestMatched++
				}
			}
		}
		name := pilotNames[code]
		if name == "" {
			name = l.Name
		}
		c := kricColor(code)
		routes = append(routes, []string{"K_" + code, "A_" + l.Opr, name, name, "1", c, textColor(c)})
	}
	for _, t := range tt.Trains {
		l := tt.Lines[t.Line]
		var rows [][]string
		prevDep := ""
		monotonic := true
		firstOrder, lastOrder := 0, 0
		for _, s := range t.Stops {
			id, ok := stopOf[t.Line][s.Code]
			if !ok {
				st.SkippedStops++
				continue
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
			if len(rows) == 0 {
				firstOrder = stationOf[t.Line][s.Code].Order
			}
			lastOrder = stationOf[t.Line][s.Code].Order
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
		service := "WEEKDAY"
		switch t.Day {
		case "7":
			service = "SAT"
		case "9":
			service = "SUN"
			if !l.HasSat {
				service = "SATSUN"
			}
		}
		dir := "0"
		if lastOrder < firstOrder {
			dir = "1"
		}
		headsign := stationOf[t.Line][t.Tmn].Name
		if headsign == "" {
			headsign = stationOf[t.Line][t.Stops[len(t.Stops)-1].Code].Name
		}
		tripID := "K_" + t.Line + "_" + service + "_" + t.No
		trips = append(trips, []string{"K_" + t.Line, service, tripID, headsign, dir})
		for i, r := range rows {
			r[0] = tripID
			r[4] = itoa(i + 1)
			stopTimes = append(stopTimes, r)
		}
		st.Trips++
	}
	return routes, trips, stopTimes, st
}

// matchPilotStop 은 레일포털 역을 파일럿 stop 에 붙인다. 이름(괄호 앞) 일치 → 그중 최근접, 없으면 KricNearestM 안 최근접.
func matchPilotStop(s kric.Station, cands []pilotStop) (id string, nearest, ok bool) {
	base := baseName(s.Name)
	bestD := -1.0
	for _, c := range cands {
		if baseName(c.Name) != base {
			continue
		}
		d := haversineM(s.Lat, s.Lon, c.Lat, c.Lon)
		if bestD < 0 || d < bestD {
			bestD, id = d, c.ID
		}
	}
	if id != "" {
		return id, false, true
	}
	if s.Lat == 0 && s.Lon == 0 {
		return "", false, false
	}
	bestD = KricNearestM
	for _, c := range cands {
		if d := haversineM(s.Lat, s.Lon, c.Lat, c.Lon); d < bestD {
			bestD, id = d, c.ID
		}
	}
	return id, id != "", id != ""
}
