package speed

// PriorTrips 5: 사전값을 trip 5개 무게로 둔다(계획 v2). 첫 trip 이 프로파일을 1/6 만 움직여 튀는 표본에 둔감하다.
const PriorTrips = 5.0

// Shrink 는 사용자 프로파일 속도 = (n0·μ0 + Σ v_trip) / (n0 + n). prior 는 수단 사전값(route.DefaultWalk/Bike).
func Shrink(prior, sum float64, n int) float64 {
	return (PriorTrips*prior + sum) / (PriorTrips + float64(n))
}
