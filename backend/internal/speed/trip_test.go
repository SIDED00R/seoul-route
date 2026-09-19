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

// 도보 구간에서는 활동 인식이 walk 인 샘플만 센다. still·unknown·vehicle 은 Mismatch 로 빠지고,
// 활동 인식이 없는 샘플(빈 값)은 그대로 센다.
func TestTripSpeedsExcludesActivityMismatch(t *testing.T) {
	t0 := time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	s := walkSamples(t0, "walk", 1.4, 5, 31, 10) // 30쌍
	for i := range s {
		switch {
		case i < 10:
			s[i].Activity = "walk" // 쌍 (0,1)~(9,10) 중 (9,10) 은 10 이 still 이라 빠진다
		case i < 13:
			s[i].Activity = "still"
		case i < 15:
			s[i].Activity = "unknown"
		case i < 25:
			s[i].Activity = "" // 활동 인식 없음 — 그대로 센다
		default:
			s[i].Activity = "vehicle"
		}
	}
	// 남는 쌍: walk 구간 9개 + 빈 값 구간 9개 = 18. 빠지는 쌍: still·unknown 경계 11개 + vehicle 경계 6개 = 17 중
	// 연속 쌍 30개에서 18을 뺀 12개가 Mismatch 다.
	w := TripSpeeds(s)["walk"]
	if !w.OK || w.Pairs != 18 || w.Mismatch != 12 || math.Abs(w.SpeedMps-1.4) > 0.01 {
		t.Fatalf("walk=%+v", w)
	}
	// 전부 불일치면 추정은 없고 Mismatch 만 남는다
	for i := range s {
		s[i].Activity = "bicycle"
	}
	w = TripSpeeds(s)["walk"]
	if w.OK || w.Pairs != 0 || w.Mismatch != 30 {
		t.Fatalf("walk=%+v", w)
	}
	// 간격이 30초를 넘는 쌍은 애초에 연속 쌍이 아니라 불일치로도 세지 않는다(키도 생기지 않는다)
	gap := []Sample{{TS: t0, Lat: 37.5, Lon: 127.0, AccuracyM: 5, Mode: "walk", Activity: "vehicle"},
		{TS: t0.Add(2 * time.Minute), Lat: 37.501, Lon: 127.0, AccuracyM: 5, Mode: "walk", Activity: "vehicle"}}
	if est := TripSpeeds(gap); len(est) != 0 {
		t.Fatalf("간격 초과 쌍이 불일치로 세였다: %+v", est)
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

// 활동 인식이 내내 정지·미상이면 그 trip 은 속도를 내지 않는다. 자전거도 같다.
func TestTripSpeedsNoSpeedFromStillOrUnknownOnlyTrip(t *testing.T) {
	t0 := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct{ mode, activity string }{
		{"walk", "still"}, {"walk", "unknown"}, {"bicycle", "still"}, {"bicycle", "unknown"},
	} {
		s := walkSamples(t0, c.mode, 2.0, 5, 40, 10)
		for i := range s {
			s[i].Activity = c.activity
		}
		if e := TripSpeeds(s)[c.mode]; e.OK || e.Pairs != 0 {
			t.Errorf("%s/%s: %+v", c.mode, c.activity, e)
		}
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
