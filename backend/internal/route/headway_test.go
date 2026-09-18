package route

import (
	"context"
	"testing"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
	"github.com/SIDED00R/seoul-route/backend/internal/routestyle"
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

// 대중교통 leg 에는 노선 색이 붙고, 표에 없는 노선·도보 leg 는 빈 값 그대로다(앱이 기본 팔레트를 쓴다).
func TestPlanAnnotatesRouteStyles(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:40:00+09:00",
			otp.Leg{Mode: "WALK"},
			otp.Leg{Mode: "SUBWAY", Route: "2호선", RouteID: "seoul:M_2", TransitLeg: true},
			otp.Leg{Mode: "BUS", Route: "999", RouteID: "seoul:B_999", TransitLeg: true})}, nil
	}}
	p := &Planner{OTP: f, RouteStyles: map[string]routestyle.Style{"M_2": {Color: "00A84D", TextColor: "FFFFFF"}}}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil {
		t.Fatal(err)
	}
	legs := its[0].Legs
	if legs[1].Color != "00A84D" || legs[1].TextColor != "FFFFFF" {
		t.Errorf("지하철 색: %+v", legs[1])
	}
	if legs[0].Color != "" || legs[2].Color != "" {
		t.Errorf("색이 없어야 하는 leg: %+v %+v", legs[0], legs[2])
	}
}

