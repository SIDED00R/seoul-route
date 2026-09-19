package geo

import (
	"math"
	"testing"
)

func near(got, want, tol float64) bool { return math.Abs(got-want) <= tol }

// 위도 0.001도 ≈ 111.2m, 서울 위도에서 경도 0.001도 ≈ 88.2m.
func TestDistAndCumulative(t *testing.T) {
	a, b, c := Point{37.5, 127.0}, Point{37.501, 127.0}, Point{37.501, 127.001}
	if d := DistM(a, b); !near(d, 111.2, 0.5) {
		t.Errorf("남북 0.001도 = %.1fm", d)
	}
	if d := DistM(b, c); !near(d, 88.2, 0.5) {
		t.Errorf("동서 0.001도 = %.1fm", d)
	}
	cum := Cumulative([]Point{a, b, c})
	if cum[0] != 0 || !near(cum[1], 111.2, 0.5) || !near(cum[2], 199.4, 1) {
		t.Errorf("cum=%v", cum)
	}
	if got := Cumulative(nil); len(got) != 0 {
		t.Errorf("빈 입력: %v", got)
	}
}

func TestProject(t *testing.T) {
	a, b := Point{37.5, 127.0}, Point{37.502, 127.0}
	d, tt := Project(Point{37.501, 127.001}, a, b) // 선분 가운데에서 동쪽으로 88m
	if !near(d, 88.2, 0.5) || !near(tt, 0.5, 0.01) {
		t.Errorf("가운데: d=%.1f t=%.2f", d, tt)
	}
	d, tt = Project(Point{37.499, 127.0}, a, b) // 시작점 앞쪽 → 시작점에 붙는다
	if !near(d, 111.2, 0.5) || tt != 0 {
		t.Errorf("앞쪽: d=%.1f t=%.2f", d, tt)
	}
	d, tt = Project(Point{37.5, 127.0}, a, a) // 길이 0 선분
	if d != 0 || tt != 0 {
		t.Errorf("점 선분: d=%.1f t=%.2f", d, tt)
	}
	if p := Lerp(a, b, 0.25); !near(p.Lat, 37.5005, 1e-9) || p.Lon != 127.0 {
		t.Errorf("Lerp=%v", p)
	}
}
