package railpath

import (
	"reflect"
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/geo"
	"github.com/SIDED00R/seoul-route/gtfs/internal/osm"
)

func way(id int64, nodes ...osm.RailNode) osm.RailWay {
	return osm.RailWay{Ref: "2", ID: id, Nodes: nodes}
}

func n(id int64, lat, lon float64) osm.RailNode { return osm.RailNode{ID: id, Lat: lat, Lon: lon} }

// 남북으로 곧은 선로가 중간(37.505)에서 동쪽으로 꺾였다가 돌아온다. 역 둘은 선로 양 끝 근처.
func TestPathFollowsTrack(t *testing.T) {
	g := NewGraph([]osm.RailWay{
		way(1, n(1, 37.500, 127.000), n(2, 37.505, 127.000)),
		way(2, n(2, 37.505, 127.000), n(3, 37.505, 127.003), n(4, 37.510, 127.003)),
	})
	a, b := geo.Point{Lat: 37.5001, Lon: 127.0002}, geo.Point{Lat: 37.5099, Lon: 127.0028}
	pts, ok := g.Path(a, b)
	if !ok {
		t.Fatal("경로를 못 찾았다")
	}
	cum := geo.Cumulative(pts)
	if l := cum[len(cum)-1]; l < 1250 || l > 1400 { // 556 + 265 + 556 = 1,377m 에서 양 끝 붙는 자리만큼 짧다
		t.Errorf("길이 %.0fm", l)
	}
	corner := false
	for i, p := range pts {
		if i > 0 && cum[i]-cum[i-1] > StepM+1 {
			t.Errorf("점 간격 %.0fm", cum[i]-cum[i-1])
		}
		if geo.DistM(p, geo.Point{Lat: 37.505, Lon: 127.003}) < 1 {
			corner = true
		}
	}
	if !corner {
		t.Error("선로의 꺾이는 점을 지나지 않는다(직선으로 이었다)")
	}
	again, _ := g.Path(a, b)
	if !reflect.DeepEqual(pts, again) {
		t.Error("같은 입력에 다른 경로")
	}
}

// 상·하행 선로는 서로 이어져 있지 않다. 두 역이 서로 다른 선로에 더 가까워도 한 선로로 잇는다.
func TestPathPicksOneOfParallelTracks(t *testing.T) {
	g := NewGraph([]osm.RailWay{
		way(1, n(1, 37.500, 127.0000), n(2, 37.510, 127.0000)),
		way(2, n(3, 37.500, 127.0001), n(4, 37.510, 127.0001)),
	})
	pts, ok := g.Path(geo.Point{Lat: 37.501, Lon: 126.9999}, geo.Point{Lat: 37.509, Lon: 127.0002})
	if !ok {
		t.Fatal("경로를 못 찾았다")
	}
	for _, p := range pts {
		if p.Lon != pts[0].Lon {
			t.Fatalf("선로를 건너뛰었다: %v", pts)
		}
	}
}

func TestPathFailures(t *testing.T) {
	g := NewGraph([]osm.RailWay{
		way(1, n(1, 37.500, 127.000), n(2, 37.502, 127.000)),
		way(2, n(3, 37.508, 127.000), n(4, 37.510, 127.000)), // 첫 선로와 끊겨 있다
	})
	if _, ok := g.Path(geo.Point{Lat: 37.5, Lon: 127.0}, geo.Point{Lat: 37.51, Lon: 127.0}); ok {
		t.Error("끊긴 선로인데 경로가 나왔다")
	}
	if _, ok := g.Path(geo.Point{Lat: 37.5, Lon: 127.0}, geo.Point{Lat: 37.6, Lon: 127.0}); ok {
		t.Error("도착 역 근처에 선로가 없는데 경로가 나왔다")
	}
	// 두 역은 300m 떨어져 있는데 선로는 3km 를 돌아간다 → 우회 상한에 걸린다.
	loop := NewGraph([]osm.RailWay{way(1, n(1, 37.500, 127.000), n(2, 37.513, 127.000), n(3, 37.513, 127.0034),
		n(4, 37.500, 127.0034))})
	if _, ok := loop.Path(geo.Point{Lat: 37.5, Lon: 127.0}, geo.Point{Lat: 37.5, Lon: 127.0034}); ok {
		t.Error("우회 상한을 넘는 경로가 나왔다")
	}
}
