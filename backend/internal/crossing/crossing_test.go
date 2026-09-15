package crossing

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Google 예제: (38.5,-120.2) (40.7,-120.95) (43.252,-126.453) ↔ "_p~iF~ps|U_ulLnnqC_mqNvxq`@"
func TestDecodePolyline(t *testing.T) {
	pts := DecodePolyline("_p~iF~ps|U_ulLnnqC_mqNvxq`@")
	if len(pts) != 3 || math.Abs(pts[0].Lat-38.5) > 1e-9 || math.Abs(pts[0].Lon+120.2) > 1e-9 ||
		math.Abs(pts[2].Lat-43.252) > 1e-9 || math.Abs(pts[2].Lon+126.453) > 1e-9 {
		t.Fatalf("pts=%v", pts)
	}
	if got := DecodePolyline(""); len(got) != 0 {
		t.Fatalf("빈 문자열: %v", got)
	}
	if got := DecodePolyline("_p~iF"); len(got) != 0 { // 위도만 있고 경도가 끊김
		t.Fatalf("잘린 입력은 그때까지만: %v", got)
	}
}

func TestExpectedWait(t *testing.T) {
	if ExpectedWaitSec != 38 { // (130−30)²/(2·130) = 38.46 → 38
		t.Fatalf("ExpectedWaitSec=%v", ExpectedWaitSec)
	}
}

// 테헤란로 방향(동서) 300m 도보 경로. 위도 1e-5 도 ≈ 1.1m, 경도 1e-5 도 ≈ 0.89m(서울).
func TestCountAlong(t *testing.T) {
	path := []Point{{37.5000, 127.0300}, {37.5000, 127.0310}, {37.5000, 127.0320}, {37.5000, 127.0334}}
	ix := New([]Point{
		{37.50003, 127.0305},  // 경로에서 3.3m → 셈
		{37.50010, 127.0315},  // 11m → 안 셈
		{37.50002, 127.0325},  // 2.2m → 셈
		{37.49998, 127.03258}, // 위 노드에서 약 8m(중앙분리대 반대편) → 병합
		{37.50000, 127.0400},  // 경로 밖(멀리) → 안 셈
		{37.5000, 127.0334},   // 경로 끝점 위 → 셈
	})
	if got := ix.CountAlong(path); got != 3 {
		t.Fatalf("count=%d want 3", got)
	}
	if got := ix.CountAlong(path[:1]); got != 0 {
		t.Fatalf("점 하나면 0: %d", got)
	}
	var nilIx *Index
	if got := nilIx.CountAlong(path); got != 0 {
		t.Fatalf("색인 없으면 0: %d", got)
	}
	// 같은 노드를 두 구간이 다 스치더라도 한 번만 센다
	ix2 := New([]Point{{37.5000, 127.0310}})
	if got := ix2.CountAlong(path); got != 1 {
		t.Fatalf("정점 위 노드 중복: %d", got)
	}
}

func TestLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.csv")
	os.WriteFile(p, []byte("lat,lon,osm_id\n37.5,127.0,1\n37.6,127.1,2\n"), 0o644)
	ix, err := Load(p)
	if err != nil || ix.Len() != 2 {
		t.Fatalf("len=%v err=%v", ix, err)
	}
	if ix.Count("") != 0 {
		t.Fatal("빈 폴리라인은 0")
	}
	os.WriteFile(p, []byte("x,y\n1,2\n"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("lat/lon 열이 없으면 오류")
	}
}
