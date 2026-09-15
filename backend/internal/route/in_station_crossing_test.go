package route

import (
	"context"
	"testing"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 역 안 환승 통로 도보(InStation)는 폴리라인이 지상 횡단보도에 걸려도 대기를 더하지 않고, 역 밖 도보는 그대로 더한다(이슈 #57).
func TestInStationWalkGetsNoCrossingWait(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		passage := walkLeg("passage", "2026-09-14T14:10:00+09:00", "2026-09-14T14:12:00+09:00", 37.48, 126.98)
		passage.InStation = true
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:40:00+09:00",
			walkLeg("street", "2026-09-14T14:00:00+09:00", "2026-09-14T14:03:00+09:00", 37.49, 126.98),
			otp.Leg{Mode: "SUBWAY", Route: "4호선", TransitLeg: true, Start: "2026-09-14T14:05:00+09:00",
				End: "2026-09-14T14:10:00+09:00"},
			passage,
			otp.Leg{Mode: "SUBWAY", Route: "2호선", TransitLeg: true, Start: "2026-09-14T14:20:00+09:00",
				End: "2026-09-14T14:40:00+09:00"})}, nil
	}}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"street": 1, "passage": 2}, CrossingSec: 38}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 1 {
		t.Fatalf("err=%v its=%d", err, len(its))
	}
	it := its[0]
	if it.Legs[2].Crossings != 0 || it.Legs[2].CrossingWait != 0 || it.Legs[2].Duration != 300 {
		t.Errorf("역 안 통로에 대기가 붙었다: %+v", it.Legs[2])
	}
	if it.Legs[0].Crossings != 1 || it.CrossingWait != 38 || it.Duration != 2400+38 {
		t.Errorf("역 밖 도보만 38초: leg0=%+v total wait=%v dur=%v", it.Legs[0], it.CrossingWait, it.Duration)
	}
}
