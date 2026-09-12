package route

import (
	"context"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 실시간 보정기: 402번 후보에 +10분을 얹는다.
type fakeRealtime struct{ calls int }

func (f *fakeRealtime) Adjust(_ context.Context, its []otp.Itinerary) []otp.Itinerary {
	f.calls++
	out := append([]otp.Itinerary(nil), its...)
	for i := range out {
		if len(out[i].Legs) > 0 && out[i].Legs[0].Route == "402" {
			out[i].Duration += 600
			out[i].Realtime, out[i].RealtimeDelta = true, 600
		}
	}
	return out
}

// 실시간 보정은 순위가 정해진 뒤 적용되고, 바뀐 소요시간으로 다시 정렬된다. 미래 출발에는 적용하지 않는다.
func TestPlanAppliesRealtimeThenReranks(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{
			itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:38:00+09:00",
				otp.Leg{Mode: "BUS", Route: "402", TransitLeg: true}),
			itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:40:00+09:00",
				otp.Leg{Mode: "SUBWAY", Route: "서울4호선", TransitLeg: true}),
		}, nil
	}}
	rt := &fakeRealtime{}
	p := &Planner{OTP: f, Realtime: rt}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil {
		t.Fatal(err)
	}
	if rt.calls != 1 || len(its) != 2 || its[0].Legs[0].Route != "서울4호선" || !its[1].Realtime || its[1].Duration != 48*60 {
		t.Fatalf("보정 후 지하철 40분이 402(38+10분)보다 앞이어야: calls=%d %+v", rt.calls, its)
	}
	dep := time.Date(2026, 9, 14, 15, 0, 0, 0, time.FixedZone("KST", 9*3600))
	its, err = p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam, Depart: &dep})
	if err != nil || rt.calls != 1 || its[0].Legs[0].Route != "402" {
		t.Fatalf("미래 출발은 실시간 미적용: calls=%d %+v err=%v", rt.calls, its, err)
	}
}

// 컷(최선+30분)은 실시간 보정 뒤에 건다: 시간표상 최선(402, 38분)보다 32분 느린 4호선 후보는 보정 전엔 컷 밖이지만,
// 402 가 실시간으로 10분 늦어지면 22분 차이라 살아남아야 한다.
func TestCutoffAppliesAfterRealtime(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{
			itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:38:00+09:00",
				otp.Leg{Mode: "BUS", Route: "402", TransitLeg: true}),
			itin("2026-09-14T14:00:00+09:00", "2026-09-14T15:10:00+09:00",
				otp.Leg{Mode: "SUBWAY", Route: "서울4호선", TransitLeg: true}),
		}, nil
	}}
	p := &Planner{OTP: f, Realtime: &fakeRealtime{}}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 2 || its[1].Legs[0].Route != "서울4호선" {
		t.Fatalf("보정 후 22분 차이인 4호선 후보가 남아야: %+v err=%v", its, err)
	}
	p.Realtime = nil // 보정이 없으면 32분 차이라 컷
	if its, _ = p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam}); len(its) != 1 {
		t.Fatalf("보정 없이는 컷: %+v", its)
	}
}
