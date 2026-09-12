package route

import (
	"context"
	"testing"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 버스 leg 에는 GTFS 배차간격이 붙고, 표에 없는 노선·도보 leg 는 0 그대로.
func TestPlanAnnotatesHeadways(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:40:00+09:00",
			otp.Leg{Mode: "WALK"},
			otp.Leg{Mode: "BUS", Route: "402", RouteID: "seoul:B_100100063", TransitLeg: true},
			otp.Leg{Mode: "SUBWAY", Route: "서울2호선", RouteID: "seoul:RR_2", TransitLeg: true})}, nil
	}}
	p := &Planner{OTP: f, Headways: map[string]int{"B_100100063": 540}}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil {
		t.Fatal(err)
	}
	legs := its[0].Legs
	if legs[0].HeadwaySec != 0 || legs[1].HeadwaySec != 540 || legs[2].HeadwaySec != 0 {
		t.Fatalf("배차 주석: %+v", legs)
	}
}
