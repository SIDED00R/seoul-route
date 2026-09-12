package route

import (
	"context"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 역으로 앵커링된 출발·도착지: OTP 요청 시각은 진입시간만큼 뒤로 밀리고, 여정 출발은 진입시간만큼 앞당겨지며(출입구 출발),
// 도착은 이탈시간만큼 늦춰진다(출입구 도착). 좌표 그대로인 쪽에는 아무것도 붙지 않는다.
func TestPlanAddsStationEntryExit(t *testing.T) {
	fixed := time.Date(2026, 9, 14, 14, 0, 0, 0, time.FixedZone("KST", 9*3600))
	f := &fakeOTP{}
	f.answer = func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-09-14T14:05:00+09:00", "2026-09-14T14:35:00+09:00",
			otp.Leg{Mode: "SUBWAY", Route: "4호선", TransitLeg: true,
				Start: "2026-09-14T14:05:00+09:00", End: "2026-09-14T14:35:00+09:00"})}, nil
	}
	p := &Planner{OTP: f, Now: func() time.Time { return fixed }}
	p.SetStations([]otp.Station{seoulStation, {ID: "seoul:ST_강남", Name: "강남", Lat: 37.4979, Lon: 127.0276}})
	origin := Point{Lat: 37.55407, Lon: 126.97070, Name: "서울역"}
	dest := Point{Lat: 37.4979, Lon: 127.0276, Name: "강남역 2호선"}
	ctx := context.Background()

	its, err := p.Plan(ctx, PlanRequest{Origin: origin, Destination: dest})
	if err != nil || len(its) != 1 {
		t.Fatalf("its=%d err=%v", len(its), err)
	}
	for _, c := range f.calls {
		if c.Depart == nil || !c.Depart.Equal(fixed.Add(StationEntrySec*time.Second)) {
			t.Fatalf("앵커링된 출발지는 요청 시각을 진입시간만큼 뒤로: %+v", c.Depart)
		}
	}
	it := its[0]
	if it.Start != "2026-09-14T14:03:00+09:00" || it.End != "2026-09-14T14:36:00+09:00" {
		t.Fatalf("출발 −2분·도착 +1분이어야: %s ~ %s", it.Start, it.End)
	}
	if it.Duration != 1800+StationEntrySec+StationExitSec || it.DepartIn != 180 {
		t.Fatalf("소요 +3분, 출발 대기 = 14:03−14:00: dur=%v wait=%v", it.Duration, it.DepartIn)
	}
	if it.Legs[0].Start != "2026-09-14T14:05:00+09:00" {
		t.Fatalf("leg 시각은 그대로(승강장 기준): %s", it.Legs[0].Start)
	}

	// 도착지가 좌표면 이탈시간 없음.
	f.calls = nil
	its, _ = p.Plan(ctx, PlanRequest{Origin: origin, Destination: gangnam})
	if its[0].Duration != 1800+StationEntrySec || its[0].End != "2026-09-14T14:35:00+09:00" {
		t.Fatalf("도착지 좌표: dur=%v end=%s", its[0].Duration, its[0].End)
	}

	// 미래 출발도 요청 시각을 밀되 출발 대기는 0.
	f.calls = nil
	dep := fixed.Add(30 * time.Minute)
	its, _ = p.Plan(ctx, PlanRequest{Origin: origin, Destination: dest, Depart: &dep})
	if d := f.calls[0].Depart; d == nil || !d.Equal(dep.Add(StationEntrySec*time.Second)) {
		t.Fatalf("미래 출발 요청 시각: %+v", d)
	}
	if its[0].DepartIn != 0 {
		t.Fatalf("미래 출발 대기는 0: %v", its[0].DepartIn)
	}

	// 도보 고정 구간은 역이라도 좌표로 요청하므로(anchorsSegment) 진입·이탈을 붙이지 않고 요청 시각도 그대로.
	f.calls = nil
	f.answer = func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-09-14T14:05:00+09:00", "2026-09-14T14:35:00+09:00", otp.Leg{Mode: "WALK"})}, nil
	}
	its, err = p.Plan(ctx, PlanRequest{Origin: origin, Destination: dest, Modes: []SegmentMode{ModeWalk}})
	if err != nil || len(its) != 1 {
		t.Fatalf("walk: its=%d err=%v", len(its), err)
	}
	if c := f.calls[0]; c.OriginStop != "" || c.Depart != nil {
		t.Fatalf("도보 고정은 좌표·요청 시각 그대로: %+v", c)
	}
	if w := its[0]; w.Duration != 1800 || w.Start != "2026-09-14T14:05:00+09:00" || w.End != "2026-09-14T14:35:00+09:00" {
		t.Fatalf("도보 고정에는 진입·이탈 없음: %+v", w)
	}
	f.answer = func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-09-14T14:05:00+09:00", "2026-09-14T14:35:00+09:00",
			otp.Leg{Mode: "SUBWAY", Route: "4호선", TransitLeg: true,
				Start: "2026-09-14T14:05:00+09:00", End: "2026-09-14T14:35:00+09:00"})}, nil
	}

	// 둘 다 좌표면 요청 시각도 그대로(nil = 지금).
	f.calls = nil
	its, _ = p.Plan(ctx, PlanRequest{Origin: seoulStn, Destination: gangnam})
	for _, c := range f.calls {
		if c.Depart != nil {
			t.Fatalf("좌표 출발지는 요청 시각 그대로: %+v", c.Depart)
		}
	}
	if its[0].Duration != 1800 || its[0].Start != "2026-09-14T14:05:00+09:00" {
		t.Fatalf("좌표끼리는 무변경: %+v", its[0])
	}
}
