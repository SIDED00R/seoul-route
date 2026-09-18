package route

import (
	"context"
	"testing"

	"github.com/SIDED00R/seoul-route/backend/internal/fastexit"
	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 지하철 leg 에는 하차역 설비 앞 칸이 붙는다. 방향은 하차 직전 정차역(없으면 탑승역)으로 정하고, 버스·도보에는 안 붙는다.
func TestPlanAnnotatesFastExits(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:40:00+09:00",
			otp.Leg{Mode: "WALK", ToName: "사당"},
			otp.Leg{Mode: "SUBWAY", Route: "2호선", FromName: "서울대입구(2호선)", ToName: "사당(2호선)", TransitLeg: true,
				Stops: []otp.Stop{{Name: "낙성대"}}},
			otp.Leg{Mode: "SUBWAY", Route: "2호선", FromName: "방배", ToName: "사당(2호선)", TransitLeg: true},
			otp.Leg{Mode: "BUS", Route: "2호선", FromName: "낙성대", ToName: "사당", TransitLeg: true})}, nil
	}}
	ix := fastexit.NewIndex([]fastexit.Row{
		{Line: "2호선", Station: "사당", Side: "상행", Toward: "방배", Door: "3-3", Facility: "계단"},
		{Line: "2호선", Station: "사당", Side: "하행", Toward: "낙성대", Door: "5-2", Facility: "계단"},
	}, nil)
	p := &Planner{OTP: f, FastExits: ix}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil {
		t.Fatal(err)
	}
	legs := its[0].Legs
	if len(legs[1].FastExit) != 1 || legs[1].FastExit[0].Doors[0] != "3-3" { // 낙성대를 지나옴 → 방배 방면 승강장
		t.Errorf("중간 정차 기준: %+v", legs[1].FastExit)
	}
	if len(legs[2].FastExit) != 1 || legs[2].FastExit[0].Doors[0] != "5-2" { // 한 정거장: 탑승역 방배 기준
		t.Errorf("탑승역 기준: %+v", legs[2].FastExit)
	}
	if legs[0].FastExit != nil || legs[3].FastExit != nil {
		t.Errorf("지하철이 아닌 leg: %+v %+v", legs[0].FastExit, legs[3].FastExit)
	}
	p.FastExits = nil
	its, _ = p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if its[0].Legs[1].FastExit != nil {
		t.Error("자료가 없는데 붙었다")
	}
}
