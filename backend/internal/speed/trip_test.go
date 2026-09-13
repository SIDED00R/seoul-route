package speed

import (
	"math"
	"testing"
	"time"
)

// 북쪽으로 v m/s 로 dt 초마다 찍은 샘플. 위도 1도 ≈ 111.2km.
func walkSamples(t0 time.Time, mode string, v, dt float64, n int, acc float64) []Sample {
	var out []Sample
	for i := 0; i < n; i++ {
		out = append(out, Sample{TS: t0.Add(time.Duration(float64(i) * dt * float64(time.Second))),
			Lat: 37.5 + v*dt*float64(i)/111195, Lon: 127.0, AccuracyM: acc, Mode: mode})
	}
	return out
}

func TestTripSpeedsMedianOfMovingPairs(t *testing.T) {
	t0 := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	s := walkSamples(t0, "walk", 1.4, 5, 20, 10) // 19쌍 1.4 m/s
	// 정지 3쌍(횡단보도): 같은 좌표가 이어진다 → 이동 중이 아니라 제외
	last := s[len(s)-1]
	for i := 1; i <= 3; i++ {
		s = append(s, Sample{TS: last.TS.Add(time.Duration(i) * 5 * time.Second), Lat: last.Lat, Lon: last.Lon,
			AccuracyM: 10, Mode: "walk"})
	}
	// GPS 점프 1쌍(200m/5s = 40 m/s) 과 정확도 나쁜 쌍은 제외
	jump := s[len(s)-1]
	s = append(s, Sample{TS: jump.TS.Add(5 * time.Second), Lat: jump.Lat + 200.0/111195, Lon: 127.0, AccuracyM: 10,
		Mode: "walk"})
	bad := s[len(s)-1]
	s = append(s, Sample{TS: bad.TS.Add(5 * time.Second), Lat: bad.Lat + 7.0/111195, Lon: 127.0, AccuracyM: 80,
		Mode: "walk"})
	// 순서를 섞어 넣어도 정렬한다
	s[0], s[5] = s[5], s[0]
	est := TripSpeeds(s)
	w := est["walk"]
	if !w.OK || w.Pairs != 19 || math.Abs(w.SpeedMps-1.4) > 0.01 {
		t.Fatalf("walk=%+v", w)
	}
	if _, ok := est["bicycle"]; ok {
		t.Fatal("자전거 샘플이 없는데 추정이 나왔다")
	}
}

func TestTripSpeedsModeBoundaryAndTransit(t *testing.T) {
	t0 := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	s := walkSamples(t0, "walk", 1.2, 5, 13, 5) // 12쌍 = MinPairs 경계
	// 이어서 자전거 4.0 m/s 12쌍 이후 대중교통 샘플: 수단이 바뀌는 쌍(walk→bicycle, bicycle→transit)은 세지 않는다
	b := walkSamples(s[len(s)-1].TS.Add(5*time.Second), "bicycle", 4.0, 5, 13, 5)
	for i := range b {
		b[i].Lat += 0.01
	}
	s = append(s, b...)
	last := b[len(b)-1]
	for i, d := range []float64{0.001, 0.002} {
		s = append(s, Sample{TS: last.TS.Add(time.Duration(i+1) * 5 * time.Second), Lat: last.Lat + d, Lon: 127.0,
			AccuracyM: 5, Mode: "transit"})
	}
	est := TripSpeeds(s)
	if w := est["walk"]; !w.OK || w.Pairs != 12 || math.Abs(w.SpeedMps-1.2) > 0.01 {
		t.Fatalf("walk=%+v", w)
	}
	if c := est["bicycle"]; !c.OK || c.Pairs != 12 || math.Abs(c.SpeedMps-4.0) > 0.01 {
		t.Fatalf("bicycle=%+v", c)
	}
	if _, ok := est["transit"]; ok {
		t.Fatal("transit 은 추정하지 않는다")
	}
	// 11쌍이면 OK=false 지만 값은 남긴다
	est = TripSpeeds(walkSamples(t0, "walk", 1.2, 5, 12, 5))
	if w := est["walk"]; w.OK || w.Pairs != 11 {
		t.Fatalf("경계 아래: %+v", w)
	}
}

func TestTripSpeedsGapAndBikeMovingThreshold(t *testing.T) {
	t0 := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	// 40초 간격은 쌍이 아니다(MaxPairSec 30)
	s := walkSamples(t0, "walk", 1.2, 40, 30, 5)
	if _, ok := TripSpeeds(s)["walk"]; ok {
		t.Fatal("40초 간격 쌍이 세졌다")
	}
	// 자전거 0.4 m/s 는 정지로 본다(MinMoving 0.5)
	if _, ok := TripSpeeds(walkSamples(t0, "bicycle", 0.4, 5, 30, 5))["bicycle"]; ok {
		t.Fatal("자전거 0.4 m/s 가 이동으로 세졌다")
	}
}

func TestShrink(t *testing.T) {
	// trip 0개면 사전값, trip 1개 1.8 m/s 면 (5×1.2+1.8)/6 = 1.3
	if v := Shrink(1.2, 0, 0); v != 1.2 {
		t.Fatalf("n=0: %v", v)
	}
	if v := Shrink(1.2, 1.8, 1); math.Abs(v-1.3) > 1e-9 {
		t.Fatalf("n=1: %v", v)
	}
	// trip 20개가 전부 1.8 이면 1.2 쪽으로 1/5 만 남는다: (6+36)/25 = 1.68
	if v := Shrink(1.2, 36, 20); math.Abs(v-1.68) > 1e-9 {
		t.Fatalf("n=20: %v", v)
	}
}
