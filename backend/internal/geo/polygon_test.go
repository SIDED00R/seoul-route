package geo

import (
	"math"
	"testing"
)

// 서울 위도에서 경도 0.0001° ≈ 8.8m, 위도 0.0001° ≈ 11.1m.
func square(lat, lon, half float64) [][2]float64 {
	return [][2]float64{{lon - half, lat - half}, {lon + half, lat - half}, {lon + half, lat + half},
		{lon - half, lat + half}, {lon - half, lat - half}}
}

func TestDistToPolygonM(t *testing.T) {
	const lat, lon = 37.5, 127.0
	box := [][][2]float64{square(lat, lon, 0.0002)}
	if d := DistToPolygonM(lat, lon, box); d != 0 {
		t.Errorf("안쪽 = %v, want 0", d)
	}
	// 동쪽 변(lon+0.0002)에서 경도로 0.0003 더 나간 점: 약 26.5m
	if d := DistToPolygonM(lat, lon+0.0005, box); math.Abs(d-DistM(lat, lon+0.0005, lat, lon+0.0002)) > 0.5 {
		t.Errorf("동쪽 바깥 = %v", d)
	}
	// 모서리 바깥은 꼭짓점까지의 거리
	corner := DistM(lat+0.0005, lon+0.0005, lat+0.0002, lon+0.0002)
	if d := DistToPolygonM(lat+0.0005, lon+0.0005, box); math.Abs(d-corner) > 0.5 {
		t.Errorf("모서리 바깥 = %v, want %v", d, corner)
	}
}

// ㅁ자 건물의 가운데 마당(구멍)은 건물 안이 아니다. 거리는 안쪽 벽까지.
func TestDistToPolygonMHole(t *testing.T) {
	const lat, lon = 37.5, 127.0
	withHole := [][][2]float64{square(lat, lon, 0.0004), square(lat, lon, 0.0002)}
	d := DistToPolygonM(lat, lon, withHole)
	if want := DistM(lat, lon, lat, lon+0.0002); math.Abs(d-want) > 0.5 {
		t.Errorf("마당 가운데 = %v, want %v", d, want)
	}
	if d := DistToPolygonM(lat, lon+0.0003, withHole); d != 0 {
		t.Errorf("벽 안 = %v, want 0", d)
	}
}
