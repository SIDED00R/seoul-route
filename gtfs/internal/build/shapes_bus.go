package build

import (
	"strconv"
	"strings"

	"github.com/SIDED00R/seoul-route/gtfs/internal/geo"
	"github.com/SIDED00R/seoul-route/gtfs/internal/seoulbus"
)

// 버스 경로선(shape) 상수. 2026-09-19 서울 버스 API 의 노선 경로(getRoutePath) 1,357노선·정류장 88,369개로 잰 값이다.
// 노선 경로나 정류장 좌표 출처가 바뀌면 다시 잰다.
//   - BusSnapM 100: 정류장이 경로에서 이보다 멀면 경로에서 어긋난 정류장으로 센다. 가운데 정류장이 하나도 안 어긋난
//     노선이 1,202, 하나만 어긋나고 그 거리가 300m 안인 노선이 28(경로에 아직 없는 새 정류장 — 441·240번 한강버스
//     선착장), 넷 이상 어긋나고 1km 넘게 벗어난 노선이 84(경기·인천 면허 노선은 경로가 일부 구간만 온다)로 갈린다.
//   - BusMaxOffStops 1, BusFarM 300: 한 방향에서 어긋난 가운데 정류장이 이 수 이하이고, 양 끝을 포함한 모든 정류장이
//     BusFarM 안이면 경로를 쓴다. 양 끝 정류장(차고지 기점)은 어긋난 수에 세지 않는다. 다른 조건은 다 맞는데 끝 정류장만
//     BusFarM 을 넘어 shape 를 잃는 방향이 38(그 끝 정류장들의 어긋남 최대 5.6km)이다.
//   - 탐색 창: 앞 정류장 위치에서 구간거리(fullSectDist)×BusWindowFactor+BusWindowSlackM 안에서만 다음 정류장을 찾는다.
//     왕복 노선은 같은 길을 반대로 돌아오므로 창이 없으면 맞은편 차로에 붙는다.
const (
	BusSnapM        = 100.0
	BusMaxOffStops  = 1
	BusFarM         = 300.0
	BusWindowFactor = 2.0
	BusWindowSlackM = 500.0
)

// busStopMatch 는 정류장 하나가 노선 경로 위에 붙은 자리다.
type busStopMatch struct {
	alongM float64 // 경로 시작부터 따라간 거리
	distM  float64 // 정류장에서 경로까지 거리
}

func busPathPoints(path []seoulbus.PathPoint) []geo.Point {
	pts := make([]geo.Point, 0, len(path))
	for _, p := range path {
		lat, e1 := strconv.ParseFloat(strings.TrimSpace(p.Lat), 64)
		lon, e2 := strconv.ParseFloat(strings.TrimSpace(p.Lon), 64)
		// 좌표계가 다른 값(TM 좌표가 gpsX 에 온 노선이 있다)은 버린다.
		if e1 != nil || e2 != nil || lat < 30 || lat > 45 || lon < 120 || lon > 135 {
			continue
		}
		pt := geo.Point{Lat: lat, Lon: lon}
		if n := len(pts); n > 0 && pts[n-1] == pt { // API 는 같은 점을 연달아 주기도 한다
			continue
		}
		pts = append(pts, pt)
	}
	return pts
}

// matchBusStops 는 정류장을 순서대로 경로 위에 붙인다. 붙는 자리는 앞 정류장보다 뒤로 가지 않는다.
func matchBusStops(pts []geo.Point, cum []float64, stops []seoulbus.Stop) []busStopMatch {
	out := make([]busStopMatch, len(stops))
	seg, along := 0, 0.0
	for i, s := range stops {
		p := geo.Point{Lat: parseF(s.Lat), Lon: parseF(s.Lon)}
		limit := along + BusWindowSlackM
		if i > 0 {
			limit += parseF(s.SectDist) * BusWindowFactor
		}
		best := busStopMatch{alongM: along, distM: -1}
		bestSeg := seg
		for k := seg; k < len(pts)-1 && (cum[k] <= limit || best.distM < 0); k++ {
			d, t := geo.Project(p, pts[k], pts[k+1])
			a := cum[k] + t*(cum[k+1]-cum[k])
			if a >= along && (best.distM < 0 || d < best.distM) {
				best, bestSeg = busStopMatch{alongM: a, distM: d}, k
			}
		}
		if best.distM < 0 { // 경로가 앞 정류장 자리에서 끝났다
			best.distM = geo.DistM(p, pts[len(pts)-1])
		}
		out[i] = best
		seg, along = bestSeg, best.alongM
	}
	return out
}

// busShape 는 한 방향의 기록 정류장(inside, 정류장 번호)이 지나는 경로선과 정류장별 shape_dist_traveled 를 만든다.
// 어긋난 가운데 정류장이 BusMaxOffStops 보다 많거나, 어느 정류장이든 BusFarM 보다 멀거나, 붙은 자리가 앞 정류장보다
// 나아가지 않으면 false — 그 방향은 shape 없이 둔다. 어긋난 정류장은 경로선이 그 좌표를 들렀다 가게 한다(양 끝이면 끝에 붙인다):
// OTP 는 shape_dist_traveled 자리가 정류장에서 150m 넘게 떨어진 trip 을 통째로 직선으로 되돌린다(2.10 실측).
func busShape(pts []geo.Point, cum []float64, m []busStopMatch, stops []seoulbus.Stop, inside []int) (
	shape []geo.Point, dists []float64, ok bool) {
	if len(pts) < 2 || len(inside) < 2 {
		return nil, nil, false
	}
	first, last := inside[0], inside[len(inside)-1]
	off := 0
	for n, i := range inside {
		if m[i].distM > BusFarM {
			return nil, nil, false
		}
		if m[i].distM > BusSnapM && i != first && i != last {
			if off++; off > BusMaxOffStops {
				return nil, nil, false
			}
		}
		if n > 0 && m[i].alongM <= m[inside[n-1]].alongM {
			return nil, nil, false
		}
	}
	at := make([]int, len(inside)) // 정류장마다 그 정류장 자리인 shape 점의 번호
	k := 0
	for n, i := range inside {
		for ; k < len(pts) && cum[k] < m[i].alongM; k++ {
			if cum[k] > m[first].alongM {
				shape = append(shape, pts[k])
			}
		}
		on := pointAt(pts, cum, m[i].alongM)
		stop := geo.Point{Lat: parseF(stops[i].Lat), Lon: parseF(stops[i].Lon)}
		switch {
		case m[i].distM <= BusSnapM:
			shape = append(shape, on)
			at[n] = len(shape) - 1
		case i == first:
			shape = append(shape, stop, on)
			at[n] = len(shape) - 2
		default:
			shape = append(shape, on, stop)
			at[n] = len(shape) - 1
			if i != last {
				shape = append(shape, on)
			}
		}
	}
	// 거리는 shape 점을 이어 잰 값에서 읽는다 — shapes.txt 의 shape_dist_traveled 와 같은 계산이라 마지막 정류장이
	// shape 길이를 넘지 않는다.
	shapeCum := geo.Cumulative(shape)
	dists = make([]float64, len(inside))
	for n := range inside {
		dists[n] = shapeCum[at[n]]
	}
	return shape, dists, true
}

// pointAt 은 경로를 alongM 만큼 따라간 자리의 점.
func pointAt(pts []geo.Point, cum []float64, alongM float64) geo.Point {
	for k := 1; k < len(pts); k++ {
		if alongM <= cum[k] {
			if cum[k] == cum[k-1] {
				return pts[k]
			}
			return geo.Lerp(pts[k-1], pts[k], (alongM-cum[k-1])/(cum[k]-cum[k-1]))
		}
	}
	return pts[len(pts)-1]
}

// shapeRows 는 shapes.txt 행(shape_id, lat, lon, sequence, shape_dist_traveled)을 만든다. 쓰는 자릿수에서 앞 점과 좌표가
// 같은 점(정류장이 붙은 자리가 경로의 꼭짓점과 겹칠 때)은 뺀다 — 거리가 같은 점이 연달아 나오지 않게.
func shapeRows(id string, pts []geo.Point) [][]string {
	cum := geo.Cumulative(pts)
	rows := make([][]string, 0, len(pts))
	for i, p := range pts {
		lat, lon := fmtCoord(p.Lat), fmtCoord(p.Lon)
		if n := len(rows); n > 0 && rows[n-1][1] == lat && rows[n-1][2] == lon {
			if i == len(pts)-1 { // 마지막 정류장의 거리는 빠지는 이 점의 값이다 — 남는 점이 그 거리를 갖는다
				rows[n-1][4] = fmtDist(cum[i])
			}
			continue
		}
		rows = append(rows, []string{id, lat, lon, strconv.Itoa(len(rows) + 1), fmtDist(cum[i])})
	}
	return rows
}

// fmtDist: 좌표를 소수 6자리로 쓰므로 서로 다른 두 점은 약 0.08m 이상 떨어져 있다. 0.01m 단위면 이웃 점의 거리가 같아지지 않는다.
func fmtDist(m float64) string { return strconv.FormatFloat(m, 'f', 2, 64) }
