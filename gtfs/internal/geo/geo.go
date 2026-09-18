// Package geo 는 경로선(shape) 계산에 쓰는 좌표 연산이다.
package geo

import "math"

type Point struct{ Lat, Lon float64 }

// DistM 은 두 점 사이 거리(m). 서울 범위에서 등장방형 근사.
func DistM(a, b Point) float64 {
	const r = 6371000.0
	x := (b.Lon - a.Lon) * math.Pi / 180 * math.Cos((a.Lat+b.Lat)/2*math.Pi/180)
	y := (b.Lat - a.Lat) * math.Pi / 180
	return r * math.Hypot(x, y)
}

// Cumulative 는 점마다 첫 점부터 선을 따라간 누적 거리(m).
func Cumulative(pts []Point) []float64 {
	cum := make([]float64, len(pts))
	for i := 1; i < len(pts); i++ {
		cum[i] = cum[i-1] + DistM(pts[i-1], pts[i])
	}
	return cum
}

// Project 는 p 를 선분 a-b 에 내린 점까지의 거리(m)와 그 점의 선분 위 위치(0~1).
func Project(p, a, b Point) (distM, t float64) {
	kx := math.Cos(p.Lat * math.Pi / 180)
	ax, ay := (a.Lon-p.Lon)*kx, a.Lat-p.Lat
	bx, by := (b.Lon-p.Lon)*kx, b.Lat-p.Lat
	dx, dy := bx-ax, by-ay
	if l := dx*dx + dy*dy; l > 0 {
		t = math.Max(0, math.Min(1, -(ax*dx+ay*dy)/l))
	}
	return DistM(p, Lerp(a, b, t)), t
}

// Lerp 는 a 와 b 사이 t(0~1) 위치의 점.
func Lerp(a, b Point, t float64) Point {
	return Point{a.Lat + (b.Lat-a.Lat)*t, a.Lon + (b.Lon-a.Lon)*t}
}
