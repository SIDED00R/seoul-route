package route

import (
	"math"
	"testing"
)

// 학습값은 GPS 로 잰 이동 중 평균이고 OTP 의 bicycle speed 는 평지 최대속도다. 평균을 그대로 넣으면 OTP 가
// 거기서 또 깎아 예상 시간이 실제보다 길어진다 — 변환은 나눗셈이므로 결과가 입력보다 커야 한다.
func TestBikeOTPSpeedConvertsAverageToMax(t *testing.T) {
	const avg = 3.5
	got := BikeOTPSpeed(avg)
	if got <= avg {
		t.Fatalf("평균 %v → OTP %v: 최대속도는 평균보다 커야 한다", avg, got)
	}
	if math.Abs(got*bikeEffective-avg) > 1e-9 {
		t.Fatalf("변환을 되돌리면 평균이어야 한다: %v", got*bikeEffective)
	}
	if math.Abs(BikeOTPSpeed(0)) > 1e-9 {
		t.Fatalf("0 은 0 이어야 한다(호출자가 기본값으로 처리): %v", BikeOTPSpeed(0))
	}
}
