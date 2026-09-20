package speed

// PriorTrips 는 기본 속도에 부여하는 가상 trip 수다.
const PriorTrips = 5.0

// Shrink 는 사용자 프로파일 속도 = (n0·μ0 + Σ v_trip) / (n0 + n). prior 는 수단 사전값(httpapi.Priors)이며
// 학습값과 같은 이동 중 속도다 — OTP 에 넣는 route.DefaultWalk/DefaultBike 는 이 값을 변환한 것이라 다르다.
func Shrink(prior, sum float64, n int) float64 {
	return (PriorTrips*prior + sum) / (PriorTrips + float64(n))
}
