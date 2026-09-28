package route

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 구간 요청마다 출발 시각부터 20분 걸리는 버스 한 대를 돌려준다(OTP via 호출은 따로 센다).
func busEachSegment(viaCalls *int) *fakeOTP {
	f := &fakeOTP{}
	f.answer = func(r otp.Request) ([]otp.Itinerary, error) {
		if len(r.Via) > 0 {
			*viaCalls++
			return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:30:00+09:00",
				otp.Leg{Mode: "BUS", Route: "via", FromName: "A", ToName: "B"})}, nil
		}
		dep := time.Date(2026, 9, 14, 14, 0, 0, 0, time.FixedZone("KST", 9*3600))
		if r.Depart != nil {
			dep = *r.Depart
		}
		return []otp.Itinerary{itin(dep.Format(time.RFC3339), dep.Add(20*time.Minute).Format(time.RFC3339),
			otp.Leg{Mode: "BUS", Route: "b", FromName: "x", ToName: "y", TransitLeg: true})}, nil
	}
	return f
}

// 체류가 있으면 OTP via 를 부르지 않고, 다음 구간은 도착 + 체류에 출발하며, 그 경유지 이음은 환승이 아니다.
func TestPlanViaStayDepartsAfterStay(t *testing.T) {
	viaCalls := 0
	f := busEachSegment(&viaCalls)
	stay := yeouido
	stay.StayMin = 30
	its, err := (&Planner{OTP: f}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Via: []Point{stay}})
	if err != nil {
		t.Fatal(err)
	}
	if viaCalls != 0 {
		t.Fatalf("체류가 있으면 OTP via 를 부르지 않는다: %d회", viaCalls)
	}
	if len(f.calls) != 2 || f.calls[1].Depart == nil ||
		!f.calls[1].Depart.Equal(time.Date(2026, 9, 14, 14, 50, 0, 0, time.FixedZone("KST", 9*3600))) {
		t.Fatalf("둘째 구간은 14:20 도착 + 30분 = 14:50 출발: %+v", f.calls)
	}
	if len(its) != 1 || its[0].Duration != 70*60 || its[0].Transfers != 0 {
		t.Fatalf("20분 + 체류 30분 + 20분 = 70분, 환승 0: %+v", its)
	}
	if l := its[0].Legs; l[0].StayVia != 1 || l[0].StaySec != 1800 || l[1].StayVia != 0 || l[1].StaySec != 0 {
		t.Fatalf("경유지에 닿는 첫 leg 에만 경유지 1·체류 1800초: %+v", l)
	}
}

// 체류가 없으면 지금처럼 OTP via 도 부르고, 버스→버스 경유지 이음은 환승 1회다.
func TestPlanViaWithoutStayKeepsViaSearch(t *testing.T) {
	viaCalls := 0
	f := busEachSegment(&viaCalls)
	its, err := (&Planner{OTP: f}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Via: []Point{yeouido}})
	if err != nil {
		t.Fatal(err)
	}
	if viaCalls != 3 {
		t.Fatalf("OTP via 3회(전체·지하철·버스): %d", viaCalls)
	}
	for _, it := range its {
		if it.Legs[0].Route == "b" && (it.Duration != 40*60 || it.Transfers != 1) {
			t.Fatalf("구간 결합 40분·환승 1: %+v", it)
		}
	}
}

func TestPlanRejectsBadStayAndBikeLimit(t *testing.T) {
	p := &Planner{OTP: &fakeOTP{}}
	long := yeouido
	long.StayMin = MaxStayMin + 1
	if _, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Via: []Point{long}}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("체류 상한 초과는 400: %v", err)
	}
	neg := yeouido
	neg.StayMin = -5
	if _, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Via: []Point{neg}}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("음수 체류는 400: %v", err)
	}
	if _, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		BikeLimitMin: 90}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("이용권은 60·120 만: %v", err)
	}
}
