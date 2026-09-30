package route

import (
	"context"
	"testing"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

func bikeLeg(poly, start, end string) otp.Leg {
	return otp.Leg{Mode: "BICYCLE", Polyline: poly, Start: start, End: end, Duration: 1200, Distance: 4000}
}

// 대중교통이 없는 여정은 가운데 자전거 leg 의 대기(5곳 190초)도 도착 시각에 들어간다.
func TestCrossingWaitOfMiddleBikeLegDelaysEnd(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:30:00+09:00",
			walkLeg("w1", "2026-09-14T14:00:00+09:00", "2026-09-14T14:05:00+09:00", 37.55, 126.97),
			bikeLeg("bk", "2026-09-14T14:05:00+09:00", "2026-09-14T14:25:00+09:00"),
			walkLeg("w2", "2026-09-14T14:25:00+09:00", "2026-09-14T14:30:00+09:00", 37.50, 127.03))}, nil
	}}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"bk": 5}, CrossingSec: 38}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 1 {
		t.Fatalf("err=%v its=%d", err, len(its))
	}
	if it := its[0]; it.End != "2026-09-14T14:33:10+09:00" || it.Duration != 1800+190 {
		t.Fatalf("End=%s Duration=%v", it.End, it.Duration)
	}
}

// 마지막 대중교통 뒤의 자전거 대기는 도착 시각에 들어가고, 대중교통 앞 도보 대기는 들어가지 않는다.
func TestCrossingWaitAfterLastTransitDelaysEnd(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:50:00+09:00",
			walkLeg("w1", "2026-09-14T14:00:00+09:00", "2026-09-14T14:05:00+09:00", 37.55, 126.97),
			otp.Leg{Mode: "BUS", TransitLeg: true, Start: "2026-09-14T14:10:00+09:00", End: "2026-09-14T14:25:00+09:00"},
			walkLeg("w2", "2026-09-14T14:25:00+09:00", "2026-09-14T14:28:00+09:00", 37.52, 127.0),
			bikeLeg("bk", "2026-09-14T14:28:00+09:00", "2026-09-14T14:47:00+09:00"),
			walkLeg("w3", "2026-09-14T14:47:00+09:00", "2026-09-14T14:50:00+09:00", 37.50, 127.03))}, nil
	}}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"w1": 1, "bk": 5}, CrossingSec: 38}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 1 {
		t.Fatalf("err=%v its=%d", err, len(its))
	}
	if it := its[0]; it.End != "2026-09-14T14:53:10+09:00" || it.Duration != 3000+38+190 {
		t.Fatalf("End=%s Duration=%v", it.End, it.Duration)
	}
}

// 재탐색한 후보는 탑승을 놓쳤는데 재탐색하지 못한 후보(상한 밖)와 같은 차를 타도 남는다.
func TestCrossingReplanKeptAgainstMissedUnreplanned(t *testing.T) {
	first := func(route, stop, next, end string) otp.Itinerary {
		return itin("2026-09-14T14:00:00+09:00", end,
			walkLeg("a", "2026-09-14T14:00:00+09:00", "2026-09-14T14:05:00+09:00", 37.55, 126.97),
			otp.Leg{Mode: "BUS", Route: route, FromStopID: stop, FromName: stop, ToName: next, NextStop: next,
				TransitLeg: true, Start: "2026-09-14T14:06:00+09:00", End: end})
	}
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		if r.Depart != nil && r.OriginStop != "" {
			route := map[string]string{"seoul:BS_1": "472", "seoul:BS_2": "100", "seoul:BS_3": "200"}[r.OriginStop]
			return []otp.Itinerary{itin("2026-09-14T14:06:54+09:00", "2026-09-14T14:40:00+09:00",
				otp.Leg{Mode: "BUS", Route: route, FromStopID: r.OriginStop, FromName: r.OriginStop, ToName: "X",
					NextStop: "X", TransitLeg: true, Start: "2026-09-14T14:12:00+09:00",
					End: "2026-09-14T14:40:00+09:00"})}, nil
		}
		return []otp.Itinerary{
			first("472", "seoul:BS_1", "X", "2026-09-14T14:30:00+09:00"),
			first("100", "seoul:BS_2", "X", "2026-09-14T14:32:00+09:00"),
			first("200", "seoul:BS_3", "X", "2026-09-14T14:34:00+09:00"),
			first("472", "seoul:BS_1", "Y", "2026-09-14T14:50:00+09:00"),
		}, nil
	}}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"a": 3}, CrossingSec: 38}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil {
		t.Fatal(err)
	}
	replanned472 := 0
	for _, it := range its {
		if it.Replanned && it.Legs[1].Route == "472" {
			replanned472++
		}
	}
	if len(its) != 4 || replanned472 != 1 {
		t.Fatalf("재탐색한 472 후보가 남아야 한다: its=%d replanned472=%d %+v", len(its), replanned472, its)
	}
}
