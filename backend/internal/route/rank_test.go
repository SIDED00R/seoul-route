package route

import (
	"testing"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

func mk(dur float64, transfers int, legs ...otp.Leg) otp.Itinerary {
	return otp.Itinerary{Duration: dur, Transfers: transfers, Legs: legs}
}

// 환승 1회 = 4분 손해: 2분 빠르지만 환승 1회 더 많은 후보는 뒤로 간다.
func TestRankPenalizesTransfers(t *testing.T) {
	fast := mk(38*60, 2, otp.Leg{Mode: "BUS", Route: "a"})
	simple := mk(40*60, 1, otp.Leg{Mode: "SUBWAY", Route: "b"})
	out := rank([]otp.Itinerary{fast, simple})
	if out[0].Legs[0].Route != "b" {
		t.Fatalf("환승 적은 40분이 환승 많은 38분보다 앞이어야: %+v", out)
	}
	// 환승 차이가 4분 이상의 시간 이득이면 빠른 쪽이 앞.
	fast.Duration = 35 * 60
	out = rank([]otp.Itinerary{fast, simple})
	if out[0].Legs[0].Route != "a" {
		t.Fatalf("5분 빠르면 환승 1회 차이를 넘는다: %+v", out)
	}
}

// 따릉이 대여 5분 페널티: 대여 경로 36분은 대중교통 40분보다 뒤.
func TestRankPenalizesRental(t *testing.T) {
	bike := mk(36*60, 0, otp.Leg{Mode: "WALK"}, otp.Leg{Mode: "BICYCLE", RentedBike: true}, otp.Leg{Mode: "WALK"})
	transit := mk(40*60, 0, otp.Leg{Mode: "SUBWAY", Route: "2"})
	out := rank([]otp.Itinerary{bike, transit})
	if out[0].Legs[0].Mode != "SUBWAY" {
		t.Fatalf("대여 페널티 후 대중교통이 앞이어야: %+v", out)
	}
}

// 최선보다 30분 넘게 느린 후보는 뺀다(따릉이 직행 122분 vs 지하철 38분).
func TestRankDropsFarSlower(t *testing.T) {
	out := rank([]otp.Itinerary{
		mk(122*60, 0, otp.Leg{Mode: "BICYCLE", RentedBike: true}),
		mk(38*60, 0, otp.Leg{Mode: "SUBWAY", Route: "2"}),
		mk(60*60, 1, otp.Leg{Mode: "BUS", Route: "x"}),
	})
	if len(out) != 2 || out[0].Duration != 38*60 || out[1].Duration != 60*60 {
		t.Fatalf("122분은 빠지고 38·60분만 남아야: %+v", out)
	}
	if got := rank(nil); len(got) != 0 {
		t.Fatalf("빈 입력: %v", got)
	}
}
