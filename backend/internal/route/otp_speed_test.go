package route

import (
	"math"
	"testing"
)

// 학습값은 GPS 로 잰 이동 중 평균이고 OTP 의 speed 는 평지 최대속도다. 평균을 그대로 넣으면 OTP 가 거기서 또
// 깎아 예상 시간이 실제보다 길어진다 — 변환은 나눗셈이므로 결과가 입력보다 커야 한다.
func TestOTPSpeedConvertsAverageToMax(t *testing.T) {
	for _, c := range []struct {
		name  string
		conv  func(float64) float64
		ratio float64
		avg   float64
	}{
		{"걷기", WalkOTPSpeed, walkEffective, 1.38},
		{"자전거", BikeOTPSpeed, bikeEffective, 3.5},
	} {
		got := c.conv(c.avg)
		if got <= c.avg {
			t.Fatalf("%s 평균 %v → OTP %v: 최대속도는 평균보다 커야 한다", c.name, c.avg, got)
		}
		if math.Abs(got*c.ratio-c.avg) > 1e-9 {
			t.Fatalf("%s 변환을 되돌리면 평균이어야 한다: %v", c.name, got*c.ratio)
		}
		if math.Abs(c.conv(0)) > 1e-9 {
			t.Fatalf("%s 0 은 0 이어야 한다(호출자가 기본값으로 처리): %v", c.name, c.conv(0))
		}
	}
	// 자전거가 걷기보다 훨씬 많이 깎인다 — 교차로 비용이 크고 끌고 가는 구간이 섞인다.
	if bikeEffective >= walkEffective {
		t.Fatalf("자전거 %v 가 걷기 %v 보다 덜 깎인다", bikeEffective, walkEffective)
	}
}

// 걷기 기본값은 학습 사전값(1.2)을 같은 변환에 넣은 값이다 — 프로파일이 없는 사용자와 있는 사용자가 같은
// 기준을 쓴다. 자전거는 표본이 0건이라 두 값이 아직 서로 다른 출처다(docs/speed-learning.md).
func TestDefaultWalkIsConvertedPrior(t *testing.T) {
	if math.Abs(DefaultWalk-WalkOTPSpeed(1.2)) > 0.005 {
		t.Fatalf("DefaultWalk %v ≠ 사전값 1.2 변환 %v", DefaultWalk, WalkOTPSpeed(1.2))
	}
}
