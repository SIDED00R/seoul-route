// Package speed 는 안내 궤적에서 사용자별 걷기·자전거 속도를 학습한다.
// trip 하나의 샘플 → 수단별 이동 중 중앙값(TripSpeeds) → 사용자 프로파일로 수축(Shrink).
package speed

import (
	"math"
	"sort"
	"time"
)

// 상수 출처(2026-09-13, 계획 v2 Phase 3 초기값. 실기기 궤적이 쌓이면 재보정)
//   - MaxAccuracyM 30: 도심 GPS 는 건물 반사로 50m 이상 튀는 표본이 흔하다. 그 이상은 버린다.
//   - MinPairSec 2 / MaxPairSec 30: 앱은 5초 간격으로 올린다. 30초 넘게 비면(앱 백그라운드·신호 끊김) 구간 속도가 아니다.
//   - MaxSpeed walk 3.0 / bicycle 12.0 m/s: 뛰는 사람 상한·전기자전거 상한. 그 이상은 GPS 점프.
//   - MinMoving walk 0.3 / bicycle 0.5 m/s: 그 미만은 정지(횡단보도 대기·신호). 정지는 Phase 4 횡단보도 대기가 맡으므로
//     여기서 뺀다. 그래서 이 값은 OTP 의 speed(평지 이동 속도)와 같은 의미다.
//   - MinPairs 12: 5초 간격 1분치. 그보다 적으면 그 수단은 표본 부족으로 판정하지 않는다.
const (
	MaxAccuracyM = 30.0
	MinPairSec   = 2.0
	MaxPairSec   = 30.0
	MinPairs     = 12
)

var (
	MaxSpeed  = map[string]float64{"walk": 3.0, "bicycle": 12.0}
	MinMoving = map[string]float64{"walk": 0.3, "bicycle": 0.5}
)

type Sample struct {
	TS        time.Time
	Lat, Lon  float64
	AccuracyM float64
	Mode      string // walk / bicycle / transit
}

// Estimate 는 한 trip 한 수단의 결과. Pairs 는 이동 중으로 센 연속 쌍 수, OK 는 MinPairs 를 넘겼는지.
type Estimate struct {
	SpeedMps float64
	Pairs    int
	OK       bool
}

// TripSpeeds 는 시각순 샘플에서 walk·bicycle 각각의 이동 중 중앙값 속도를 낸다. transit 샘플은 쓰지 않는다.
// 연속 쌍은 같은 수단이고 정확도가 좋고 간격이 2~30초일 때만 센다.
func TripSpeeds(samples []Sample) map[string]Estimate {
	sort.Slice(samples, func(i, j int) bool { return samples[i].TS.Before(samples[j].TS) })
	speeds := map[string][]float64{}
	for i := 1; i < len(samples); i++ {
		a, b := samples[i-1], samples[i]
		maxV, ok := MaxSpeed[b.Mode]
		if !ok || a.Mode != b.Mode || a.AccuracyM > MaxAccuracyM || b.AccuracyM > MaxAccuracyM {
			continue
		}
		dt := b.TS.Sub(a.TS).Seconds()
		if dt < MinPairSec || dt > MaxPairSec {
			continue
		}
		v := haversineM(a.Lat, a.Lon, b.Lat, b.Lon) / dt
		if v < MinMoving[b.Mode] || v > maxV {
			continue
		}
		speeds[b.Mode] = append(speeds[b.Mode], v)
	}
	out := map[string]Estimate{}
	for mode, vs := range speeds {
		sort.Float64s(vs)
		out[mode] = Estimate{SpeedMps: median(vs), Pairs: len(vs), OK: len(vs) >= MinPairs}
	}
	return out
}

func median(sorted []float64) float64 {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp, dl := (lat2-lat1)*math.Pi/180, (lon2-lon1)*math.Pi/180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}
