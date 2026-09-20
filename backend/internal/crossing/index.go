package crossing

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
)

// 경로와 8m 안의 신호를 찾고, 15m 안의 연속 신호는 하나로 합친다.
const (
	MatchRadiusM = 8.0
	MergeM       = 15.0
	CycleSec     = 130.0
	GreenSec     = 30.0
)

// ExpectedWaitSec 는 임의 시각에 도착했을 때의 신호 대기 기대값(초)이다.
var ExpectedWaitSec = math.Round((CycleSec - GreenSec) * (CycleSec - GreenSec) / (2 * CycleSec)) // 38

// Index 는 신호 횡단보도 좌표를 격자로 담는다. 격자 한 칸은 위경도 0.001도(약 111m×89m).
type Index struct {
	cells map[[2]int][]Point
	n     int
}

const cellDeg = 0.001

func cellOf(p Point) [2]int {
	return [2]int{int(math.Floor(p.Lat / cellDeg)), int(math.Floor(p.Lon / cellDeg))}
}

// New 는 점 목록으로 색인을 만든다.
func New(points []Point) *Index {
	ix := &Index{cells: map[[2]int][]Point{}}
	for _, p := range points {
		c := cellOf(p)
		ix.cells[c] = append(ix.cells[c], p)
		ix.n++
	}
	return ix
}

// Len 은 담긴 횡단보도 수.
func (ix *Index) Len() int { return ix.n }

// Load 는 crossings.csv(lat, lon, …)를 읽는다.
func Load(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: 헤더 없음: %w", path, err)
	}
	latI, lonI := -1, -1
	for i, h := range header {
		switch h {
		case "lat":
			latI = i
		case "lon":
			lonI = i
		}
	}
	if latI < 0 || lonI < 0 {
		return nil, fmt.Errorf("%s: lat/lon 열 없음", path)
	}
	var pts []Point
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		lat, e1 := strconv.ParseFloat(rec[latI], 64)
		lon, e2 := strconv.ParseFloat(rec[lonI], 64)
		if e1 != nil || e2 != nil {
			return nil, fmt.Errorf("%s: 좌표 파싱 실패: %v", path, rec)
		}
		pts = append(pts, Point{Lat: lat, Lon: lon})
	}
	return New(pts), nil
}

// Count 는 폴리라인이 MatchRadiusM 안으로 지나는 신호 횡단보도 수. 같은 노드는 한 번, MergeM 안에 붙은 노드들은
// 하나로 센다(중앙분리대 양쪽 노드).
func (ix *Index) Count(polyline string) int {
	return ix.CountAlong(DecodePolyline(polyline))
}

// CountAlong 은 점 목록(경로)에 대해 Count 와 같다.
func (ix *Index) CountAlong(path []Point) int {
	if ix == nil || len(path) < 2 {
		return 0
	}
	var hits []Point
	seenCell := map[[2]int]bool{}
	for i := 1; i < len(path); i++ {
		a, b := path[i-1], path[i]
		// 구간의 양 끝 셀 사각형(±1 셀)을 훑는다. 구간이 한 셀(약 100m)보다 길 수 있어 범위를 양 끝으로 잡는다.
		ca, cb := cellOf(a), cellOf(b)
		lo0, hi0 := minInt(ca[0], cb[0])-1, maxInt(ca[0], cb[0])+1
		lo1, hi1 := minInt(ca[1], cb[1])-1, maxInt(ca[1], cb[1])+1
		for x := lo0; x <= hi0; x++ {
			for y := lo1; y <= hi1; y++ {
				for _, p := range ix.cells[[2]int{x, y}] {
					if segmentDistM(p, a, b) > MatchRadiusM {
						continue
					}
					k := [2]int{int(p.Lat * 1e6), int(p.Lon * 1e6)}
					if seenCell[k] {
						continue
					}
					seenCell[k] = true
					hits = append(hits, p)
				}
			}
		}
	}
	// 붙어 있는 노드 병합
	n := 0
	var kept []Point
	for _, h := range hits {
		merged := false
		for _, k := range kept {
			if distM(h, k) <= MergeM {
				merged = true
				break
			}
		}
		if !merged {
			kept = append(kept, h)
			n++
		}
	}
	return n
}

// segmentDistM 은 점 p 에서 선분 ab 까지의 거리(m). 서울 위도의 등장방형 근사(오차 0.1% 미만).
func segmentDistM(p, a, b Point) float64 {
	kx := 111320.0 * math.Cos(a.Lat*math.Pi/180)
	ky := 110574.0
	px, py := (p.Lon-a.Lon)*kx, (p.Lat-a.Lat)*ky
	bx, by := (b.Lon-a.Lon)*kx, (b.Lat-a.Lat)*ky
	l2 := bx*bx + by*by
	t := 0.0
	if l2 > 0 {
		t = (px*bx + py*by) / l2
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
	}
	dx, dy := px-t*bx, py-t*by
	return math.Sqrt(dx*dx + dy*dy)
}

func distM(a, b Point) float64 { return segmentDistM(a, b, b) }

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
