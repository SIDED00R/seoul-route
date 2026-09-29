package geo

import "math"

// DistToPolygonM 은 점에서 다각형까지 거리(m)다. 점이 다각형 안이면 0.
// rings 는 GeoJSON Polygon 의 coordinates 처럼 [고리][점]{경도, 위도} 이고 첫 고리가 바깥, 나머지가 구멍이다.
// 수백 m 안의 거리만 쓰므로 점을 원점으로 한 평면(등장방형 근사)에서 잰다.
func DistToPolygonM(lat, lon float64, rings [][][2]float64) float64 {
	kx := 111320 * math.Cos(lat*math.Pi/180)
	const ky = 110540.0
	inside := false
	best := math.Inf(1)
	for _, ring := range rings {
		for i := 1; i < len(ring); i++ {
			x1, y1 := (ring[i-1][0]-lon)*kx, (ring[i-1][1]-lat)*ky
			x2, y2 := (ring[i][0]-lon)*kx, (ring[i][1]-lat)*ky
			// 짝홀 규칙: 원점에서 +x 로 쏜 반직선이 변을 몇 번 가르는지. 구멍 고리도 같이 세면 구멍 안은 바깥이 된다.
			if (y1 > 0) != (y2 > 0) && x1-(x2-x1)*y1/(y2-y1) > 0 {
				inside = !inside
			}
			best = math.Min(best, segDist(x1, y1, x2, y2))
		}
	}
	if inside {
		return 0
	}
	return best
}

// segDist 는 원점에서 선분 (x1,y1)-(x2,y2) 까지 거리다.
func segDist(x1, y1, x2, y2 float64) float64 {
	dx, dy := x2-x1, y2-y1
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = math.Max(0, math.Min(1, -(x1*dx+y1*dy)/l))
	}
	return math.Hypot(x1+t*dx, y1+t*dy)
}
