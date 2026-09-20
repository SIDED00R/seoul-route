package build

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
)

func parseF(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func fmtCoord(f float64) string { return strconv.FormatFloat(f, 'f', 6, 64) }

// StationRadiusM: 같은 기준명 stop 을 한 역으로 묶는 반경. 지하철 762 stop 실측에서 그룹 내 최대 이격은
// 서울역 434m 이고, 800m 를 넘는 건 동명이역(양평 53km)과 도봉산(1,118m) 둘뿐(2026-09-12).
const StationRadiusM = 800

// stationGroups 는 지하철 stop 을 부모역(location_type=1)으로 묶는다. 규칙: 괄호 앞 기준명이 같고 서로
// StationRadiusM 이내. 부모 좌표는 자식 평균. 반환: 부모 행(stop_id, stop_name, lat, lon), stop_id → 부모 id.
// 역 단위로 묶어 두면 OTP 가 stopLocation 요청에서 여정에 맞는 stop 을 고른다(역사 좌표 스냅 우회 방지).
func stationGroups(stops []ktdb.Row) (parents [][]string, parentOf map[string]string) {
	type cluster struct {
		ids      []string
		lat, lon float64 // 누적 합
	}
	// 입력은 ktdb.Load 의 맵 순회 결과라 순서가 매번 다르다. stop_id 로 정렬해야 동명이역(양평·도봉산)의
	// 부모 ID 접미사와 부모 좌표가 빌드마다 같다(안 그러면 API 가 캐시한 역 ID 가 다른 역을 가리킨다).
	sorted := append([]ktdb.Row(nil), stops...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i]["stop_id"] < sorted[j]["stop_id"] })
	byBase := map[string][]*cluster{}
	var order []string
	for _, s := range sorted {
		base := baseName(s["stop_name"])
		lat, lon := parseF(s["stop_lat"]), parseF(s["stop_lon"])
		var joined *cluster
		for _, c := range byBase[base] {
			n := float64(len(c.ids))
			if haversineM(lat, lon, c.lat/n, c.lon/n) <= StationRadiusM {
				joined = c
				break
			}
		}
		if joined == nil {
			if _, ok := byBase[base]; !ok {
				order = append(order, base)
			}
			joined = &cluster{}
			byBase[base] = append(byBase[base], joined)
		}
		joined.ids = append(joined.ids, s["stop_id"])
		joined.lat += lat
		joined.lon += lon
	}
	parentOf = map[string]string{}
	sort.Strings(order)
	for _, base := range order {
		for i, c := range byBase[base] {
			id := "ST_" + base
			if i > 0 {
				id += "_" + strings.Repeat("2", i) // 동명이역: 두 번째 그룹부터 접미사
			}
			n := float64(len(c.ids))
			parents = append(parents, []string{id, base, fmtCoord(c.lat / n), fmtCoord(c.lon / n)})
			for _, sid := range c.ids {
				parentOf[sid] = id
			}
		}
	}
	return parents, parentOf
}

// baseName 은 "서울(4호선)" → "서울". 괄호가 없으면 그대로.
func baseName(name string) string {
	if i := strings.Index(name, "("); i >= 0 {
		name = name[:i]
	}
	return strings.TrimSpace(name)
}

func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}
