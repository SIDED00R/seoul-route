package build

import (
	"strings"

	"github.com/SIDED00R/seoul-route/gtfs/internal/geo"
	"github.com/SIDED00R/seoul-route/gtfs/internal/osm"
	"github.com/SIDED00R/seoul-route/gtfs/internal/railpath"
)

// 도시철도 노선 → OSM 노선 관계의 ref(otp/extract_rail.py 가 뽑는 값, 2026-09-19 서울 추출본 기준). 선로를 같이 쓰는
// 관계가 여럿이면 다 적는다(공항철도 일반·직통). OSM 에서 ref 가 바뀌면 그 노선은 shape 없이 역 사이 직선으로 돌아간다 —
// 빌드 보고의 "선로를 못 찾은 역 간 구간" 수로 드러난다.
var railRefsByRoute = map[string][]string{
	"K_AP": {"공항철도", "AREX"},
	"K_GC": {"경춘"},
	"K_I1": {"인천1"},
	"K_I2": {"I2"},
	"K_KJ": {"경의·중앙"},
	"K_KK": {"경강"},
	"K_KP": {"김포 골드라인"},
	"K_SB": {"신분당"},
	"K_SD": {"수인·분당", "경의·중앙"}, // 왕십리–청량리는 경의중앙선 선로를 같이 쓴다
	"K_SL": {"Silim"},
	"K_UI": {"U"},
	"K_WS": {"W"},
}

// 파일럿에 남는 노선은 route_short_name 으로 찾는다.
var railRefsByName = map[string][]string{
	"서해선":   {"서해"},
	"GTX-A": {"GTX-A"},
}

// railRefs 는 노선의 OSM ref 들. 모르는 노선이면 nil.
func railRefs(routeID, shortName string) []string {
	if rest, ok := strings.CutPrefix(routeID, "M_"); ok { // M_<호선>, 급행은 M_<호선>_X
		return []string{strings.TrimSuffix(rest, "_X")}
	}
	if refs, ok := railRefsByRoute[routeID]; ok {
		return refs
	}
	return railRefsByName[shortName]
}

// RailStopSnapM: 선로에 붙은 자리가 역 좌표에서 이보다 멀면 shape 가 역 좌표를 거치게 한다. OTP 2.10 은
// shape_dist_traveled 자리가 역에서 150m 넘게 떨어진 trip 을 통째로 직선으로 되돌린다(2026-09-19 실측: 초지(서해선) 207m,
// 왕십리(수인분당선) 229m, 경기광주 225m). 역 좌표 출처나 OSM 추출본이 바뀌면 빌드 보고의 shape 경고 수로 다시 확인한다.
const RailStopSnapM = 100.0

type railShapeStats struct {
	Shapes       int // 만든 shape 수(노선·정차 순서가 같은 trip 은 하나를 같이 쓴다)
	StraightHops int // 선로를 못 찾아 직선으로 이은 역 간 구간(노선·역 쌍 단위)
	NoShapeTrips int // 어느 구간도 선로를 못 찾아 shape 없이 둔 trip
}

// 열 위치: trips(route_id, service_id, trip_id, trip_headsign, direction_id, shape_id),
// stop_times(trip_id, arrival_time, departure_time, stop_id, stop_sequence, shape_dist_traveled).
const (
	tripRouteCol, tripIDCol, tripShapeCol = 0, 2, 5
	stTripCol, stStopCol, stDistCol       = 0, 3, 5
)

// padRows 는 열이 모자란 행을 빈 값으로 채운다(shape 열은 생산자마다 따로 넣지 않고 마지막에 맞춘다).
func padRows(rows [][]string, n int) {
	for i, r := range rows {
		for len(r) < n {
			r = append(r, "")
		}
		rows[i] = r
	}
}

// railShapes 는 도시철도 trip 의 shape 를 만들어 trips 의 shape_id 와 stop_times 의 shape_dist_traveled 를 채우고
// shapes.txt 행을 돌려준다. 역 사이는 그 노선의 OSM 선로를 따라 잇고, 선로를 못 찾은 구간은 역 좌표를 직선으로 잇는다.
// routeNames 는 route_id → route_short_name, stopAt 은 stop_id → 좌표. stop_times 는 trip 별로 이어져 있고 정차 순이다.
func railShapes(ways []osm.RailWay, trips, stopTimes [][]string, routeNames map[string]string,
	stopAt map[string]geo.Point) (shapes [][]string, st railShapeStats) {
	waysByRef := map[string][]osm.RailWay{}
	for _, w := range ways {
		waysByRef[w.Ref] = append(waysByRef[w.Ref], w)
	}
	graphs := map[string]*railpath.Graph{}
	graphOf := func(refs []string) *railpath.Graph {
		key := strings.Join(refs, "|")
		if g, ok := graphs[key]; ok {
			return g
		}
		var all []osm.RailWay
		for _, r := range refs {
			all = append(all, waysByRef[r]...)
		}
		graphs[key] = railpath.NewGraph(all)
		return graphs[key]
	}
	type hop struct {
		pts []geo.Point
		ok  bool
	}
	hops := map[string]hop{}
	rowsOf := map[string][]int{} // trip_id → stop_times 행 번호
	for i, r := range stopTimes {
		rowsOf[r[stTripCol]] = append(rowsOf[r[stTripCol]], i)
	}
	shapeOf := map[string]string{}    // 노선+정차 순서 → shape_id("" 는 shape 없음)
	distsOf := map[string][]float64{} // shape_id → 정차별 shape_dist_traveled
	for _, t := range trips {
		routeID := t[tripRouteCol]
		if strings.HasPrefix(routeID, "B_") {
			continue
		}
		rows := rowsOf[t[tripIDCol]]
		refs := railRefs(routeID, routeNames[routeID])
		if refs == nil || len(rows) < 2 {
			st.NoShapeTrips++
			continue
		}
		ids := make([]string, len(rows))
		for n, i := range rows {
			ids[n] = stopTimes[i][stStopCol]
		}
		key := routeID + "|" + strings.Join(ids, ">")
		shapeID, seen := shapeOf[key]
		if !seen {
			g := graphOf(refs)
			var pts []geo.Point
			dists := make([]float64, len(ids))
			found := false
			for n := 1; n < len(ids); n++ {
				hk := strings.Join(refs, "|") + "|" + ids[n-1] + ">" + ids[n]
				h, ok := hops[hk]
				if !ok {
					from, to := stopAt[ids[n-1]], stopAt[ids[n]]
					h.pts, h.ok = g.Path(from, to)
					if h.ok { // 선로에 붙은 자리가 역 좌표에서 멀면 역 좌표를 거치게 한다
						if geo.DistM(from, h.pts[0]) > RailStopSnapM {
							h.pts = append([]geo.Point{from}, h.pts...)
						}
						if geo.DistM(to, h.pts[len(h.pts)-1]) > RailStopSnapM {
							h.pts = append(h.pts, to)
						}
					}
					if !h.ok {
						h.pts = []geo.Point{stopAt[ids[n-1]], stopAt[ids[n]]}
						st.StraightHops++
					}
					hops[hk] = h
				}
				found = found || h.ok
				pts = append(pts, h.pts...) // 앞 구간 끝점과 같은 점은 shapeRows 가 한 번만 쓴다
				cum := geo.Cumulative(pts)
				dists[n] = cum[len(cum)-1]
			}
			if found {
				st.Shapes++
				shapeID = "R_" + itoa(st.Shapes)
				shapes = append(shapes, shapeRows(shapeID, pts)...)
				distsOf[shapeID] = dists
			}
			shapeOf[key] = shapeID
		}
		if shapeID == "" {
			st.NoShapeTrips++
			continue
		}
		t[tripShapeCol] = shapeID
		for n, i := range rows {
			stopTimes[i][stDistCol] = fmtDist(distsOf[shapeID][n])
		}
	}
	return shapes, st
}
