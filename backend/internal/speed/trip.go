// Package speed 는 안내 궤적에서 사용자별 걷기·자전거 속도를 학습한다.
// trip 하나의 샘플 → 수단별 이동 중 중앙값(TripSpeeds) → 사용자 프로파일로 수축(Shrink).
package speed

import (
	"sort"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/geo"
)

// 정확한 연속 위치 표본만 사용하고, 수단별 이동 속도 범위를 벗어난 쌍은 제외한다.
const (
	MaxAccuracyM = 30.0
	MinPairSec   = 2.0
	MaxPairSec   = 30.0
	MinPairs     = 12
)

var (
	MaxSpeed  = map[string]float64{"walk": 3.0, "bicycle": 12.0}
	MinMoving = map[string]float64{"walk": 0.3, "bicycle": 0.5}
	// 활동 인식이 줄 수 있는 판정. 빈 값은 판정 정보가 없음을 뜻한다.
	Activities = map[string]bool{"walk": true, "bicycle": true, "vehicle": true, "still": true, "unknown": true}
)

type Sample struct {
	TS        time.Time
	Lat, Lon  float64
	AccuracyM float64
	Mode      string // walk / bicycle / transit — 안내 구간의 수단
	Activity  string // 폰 활동 인식 판정 walk / bicycle / vehicle / still / unknown, 없으면 ""
}

// Mismatch 는 활동 인식 판정이 안내 구간 수단과 다른 샘플인지. 정지(still)·미상(unknown)도 다른 것으로 본다 —
// 걷는 중으로 기록된 표본만 걷기 속도가 된다. 활동 인식이 없는 표본(빈 값)은 판단하지 않고 그대로 쓴다.
func (s Sample) Mismatch() bool { return s.Activity != "" && s.Activity != s.Mode }

// Estimate 는 한 trip 한 수단의 결과. Pairs 는 이동 중으로 센 연속 쌍 수, OK 는 MinPairs 를 넘겼는지,
// Mismatch 는 활동 불일치로 뺀 쌍 수.
type Estimate struct {
	SpeedMps float64
	Pairs    int
	Mismatch int
	OK       bool
}

// TripSpeeds 는 시각순 샘플에서 walk·bicycle 각각의 이동 중 중앙값 속도를 낸다. transit 샘플은 쓰지 않는다.
// 연속 쌍은 같은 수단이고 정확도가 좋고 간격이 2~30초일 때만 센다. 한쪽이라도 활동 불일치면 세지 않고 Mismatch 에 더한다.
func TripSpeeds(samples []Sample) map[string]Estimate {
	sort.Slice(samples, func(i, j int) bool { return samples[i].TS.Before(samples[j].TS) })
	speeds := map[string][]float64{}
	mismatch := map[string]int{}
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
		if a.Mismatch() || b.Mismatch() {
			mismatch[b.Mode]++
			continue
		}
		v := geo.DistM(a.Lat, a.Lon, b.Lat, b.Lon) / dt
		if v < MinMoving[b.Mode] || v > maxV {
			continue
		}
		speeds[b.Mode] = append(speeds[b.Mode], v)
	}
	out := map[string]Estimate{}
	for mode, vs := range speeds {
		sort.Float64s(vs)
		out[mode] = Estimate{SpeedMps: median(vs), Pairs: len(vs), Mismatch: mismatch[mode], OK: len(vs) >= MinPairs}
	}
	for mode, n := range mismatch {
		if _, has := out[mode]; !has {
			out[mode] = Estimate{Mismatch: n}
		}
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
