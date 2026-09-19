// Package railpath 는 OSM 선로로 그래프를 만들어 두 역 사이를 선로를 따라 잇는다.
package railpath

import (
	"container/heap"
	"math"

	"github.com/SIDED00R/seoul-route/gtfs/internal/geo"
	"github.com/SIDED00R/seoul-route/gtfs/internal/osm"
)

// 상수(2026-09-19 초기값). 서울 GTFS 의 도시철도 역 간 구간 1,733개(노선·역 쌍)로 확인: 양 끝이 OSM 추출 범위 안인 구간은
// 전부 경로를 찾았고 우회 상한에 걸린 구간은 0 이다. OSM 추출본이나 역 좌표 출처가 바뀌면 다시 센다.
//   - StepM 40: 선로 선분을 이 간격으로 쪼갠다. 역을 가장 가까운 점에 붙이므로 붙는 위치 오차가 이 값의 절반 안이다.
//   - SnapM 250: 역 좌표에서 이 반경 안의 선로 점을 출발·도착 후보로 본다(역 좌표는 승강장 중심이 아닐 수 있다).
//   - SnapPenalty 3: 역에서 후보 점까지의 거리에 곱한다. 상·하행 선로 중 어느 쪽에 붙을지는 전체 비용으로 정한다.
//   - 우회 상한: 경로 길이가 직선거리×DetourFactor+DetourSlackM 을 넘으면 다른 선로로 돌아간 것으로 보고 버린다.
const (
	StepM        = 40.0
	SnapM        = 250.0
	SnapPenalty  = 3.0
	DetourFactor = 2.5
	DetourSlackM = 800.0
)

type edge struct {
	to int
	w  float64
}

// Graph 는 한 노선(또는 선로를 같이 쓰는 노선 묶음)의 선로 그래프다. 방향은 가리지 않는다.
type Graph struct {
	pts  []geo.Point
	adj  [][]edge
	grid map[[2]int][]int
}

const cellDeg = 0.003 // 격자 한 칸(위도 약 330m). SnapM 보다 커서 이웃 9칸이면 후보를 다 본다

// NewGraph 는 선로들을 StepM 간격으로 쪼개 그래프를 만든다. OSM 노드 ID 가 같으면 같은 정점이다.
func NewGraph(ways []osm.RailWay) *Graph {
	g := &Graph{grid: map[[2]int][]int{}}
	byID := map[int64]int{}
	vertex := func(n osm.RailNode) int {
		if i, ok := byID[n.ID]; ok {
			return i
		}
		byID[n.ID] = g.add(geo.Point{Lat: n.Lat, Lon: n.Lon})
		return byID[n.ID]
	}
	for _, w := range ways {
		for i := 1; i < len(w.Nodes); i++ {
			a, b := w.Nodes[i-1], w.Nodes[i]
			prev := vertex(a)
			last := vertex(b)
			pa, pb := geo.Point{Lat: a.Lat, Lon: a.Lon}, geo.Point{Lat: b.Lat, Lon: b.Lon}
			n := int(math.Ceil(geo.DistM(pa, pb) / StepM))
			for k := 1; k < n; k++ {
				cur := g.add(geo.Lerp(pa, pb, float64(k)/float64(n)))
				g.link(prev, cur)
				prev = cur
			}
			g.link(prev, last)
		}
	}
	return g
}

func (g *Graph) add(p geo.Point) int {
	g.pts = append(g.pts, p)
	g.adj = append(g.adj, nil)
	i := len(g.pts) - 1
	c := cellOf(p)
	g.grid[c] = append(g.grid[c], i)
	return i
}

func (g *Graph) link(a, b int) {
	if a == b {
		return
	}
	w := geo.DistM(g.pts[a], g.pts[b])
	g.adj[a] = append(g.adj[a], edge{b, w})
	g.adj[b] = append(g.adj[b], edge{a, w})
}

func cellOf(p geo.Point) [2]int {
	return [2]int{int(math.Floor(p.Lat / cellDeg)), int(math.Floor(p.Lon / cellDeg))}
}

// near 는 p 에서 SnapM 안에 있는 정점과 그 거리.
func (g *Graph) near(p geo.Point) map[int]float64 {
	out := map[int]float64{}
	c := cellOf(p)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			for _, i := range g.grid[[2]int{c[0] + dy, c[1] + dx}] {
				if d := geo.DistM(p, g.pts[i]); d <= SnapM {
					out[i] = d
				}
			}
		}
	}
	return out
}

// Path 는 a 에서 b 까지 선로를 따라가는 점들을 돌려준다. 양쪽 역 근처에 선로가 없거나, 이어지지 않거나, 우회 상한을
// 넘으면 false.
func (g *Graph) Path(a, b geo.Point) ([]geo.Point, bool) {
	src, dst := g.near(a), g.near(b)
	if len(src) == 0 || len(dst) == 0 {
		return nil, false
	}
	dist := map[int]float64{}
	from := map[int]int{}
	pq := &queue{}
	for i, d := range src {
		dist[i] = d * SnapPenalty
		from[i] = -1
		heap.Push(pq, item{i, dist[i]})
	}
	goal, best := -1, math.Inf(1)
	done := map[int]bool{}
	for pq.Len() > 0 {
		it := heap.Pop(pq).(item)
		if done[it.v] {
			continue
		}
		done[it.v] = true
		if it.cost >= best {
			break
		}
		if d, ok := dst[it.v]; ok && it.cost+d*SnapPenalty < best {
			goal, best = it.v, it.cost+d*SnapPenalty
		}
		for _, e := range g.adj[it.v] {
			c := it.cost + e.w
			if old, ok := dist[e.to]; !ok || c < old {
				dist[e.to] = c
				from[e.to] = it.v
				heap.Push(pq, item{e.to, c})
			}
		}
	}
	if goal < 0 {
		return nil, false
	}
	var rev []geo.Point
	for v := goal; v >= 0; v = from[v] {
		rev = append(rev, g.pts[v])
	}
	path := make([]geo.Point, len(rev))
	for i, p := range rev {
		path[len(rev)-1-i] = p
	}
	if cum := geo.Cumulative(path); cum[len(cum)-1] > geo.DistM(a, b)*DetourFactor+DetourSlackM {
		return nil, false
	}
	return path, true
}

type item struct {
	v    int
	cost float64
}

type queue []item

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	return q[i].cost < q[j].cost || (q[i].cost == q[j].cost && q[i].v < q[j].v) // 같은 비용이면 정점 번호순(빌드마다 같은 결과)
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any)   { *q = append(*q, x.(item)) }
func (q *queue) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}
