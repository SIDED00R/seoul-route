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

// 앵커링된 출발지라도 승강장에 가지 않는 후보(출입구에서 걷기 시작하는 도보·따릉이·버스)에는 진입시간을 붙이지 않는다.
// 대중교통이 없는 후보는 밀린 요청 시각(now+진입)만큼 leg 를 앞당겨 지금 출발하게 한다.
func TestStationEntryOnlyForPlatformBoarding(t *testing.T) {
	fixed := time.Date(2026, 9, 14, 14, 0, 0, 0, time.FixedZone("KST", 9*3600))
	f := &fakeOTP{}
	f.answer = func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{
			itin("2026-09-14T14:05:00+09:00", "2026-09-14T14:35:00+09:00",
				otp.Leg{Mode: "SUBWAY", Route: "4호선", TransitLeg: true, FromName: "서울(4호선)",
					Start: "2026-09-14T14:05:00+09:00", End: "2026-09-14T14:35:00+09:00"}),
			itin("2026-09-14T14:02:00+09:00", "2026-09-14T14:40:00+09:00",
				otp.Leg{Mode: "WALK", FromName: "서울 4번 출구", Start: "2026-09-14T14:02:00+09:00", End: "2026-09-14T14:08:00+09:00"},
				otp.Leg{Mode: "BUS", Route: "401", TransitLeg: true, Start: "2026-09-14T14:10:00+09:00",
					End: "2026-09-14T14:40:00+09:00"}),
			itin("2026-09-14T14:02:00+09:00", "2026-09-14T14:32:00+09:00",
				otp.Leg{Mode: "WALK", FromName: "서울 9번 출구", Start: "2026-09-14T14:02:00+09:00", End: "2026-09-14T14:05:00+09:00"},
				otp.Leg{Mode: "BICYCLE", Start: "2026-09-14T14:05:00+09:00", End: "2026-09-14T14:32:00+09:00"}),
		}, nil
	}
	p := &Planner{OTP: f, Now: func() time.Time { return fixed }}
	p.SetStations([]otp.Station{seoulStation})
	origin := Point{Lat: 37.55407, Lon: 126.97070, Name: "서울역"}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: origin, Destination: gangnam})
	if err != nil || len(its) != 3 {
		t.Fatalf("its=%d err=%v", len(its), err)
	}
	byLast := map[string]otp.Itinerary{}
	for _, it := range its {
		byLast[it.Legs[len(it.Legs)-1].Mode] = it
	}
	sub, bus, bike := byLast["SUBWAY"], byLast["BUS"], byLast["BICYCLE"]
	if sub.Start != "2026-09-14T14:03:00+09:00" || sub.Duration != 1800+StationEntrySec {
		t.Fatalf("승강장에서 타는 지하철: 진입 2분: %s dur=%v", sub.Start, sub.Duration)
	}
	if bus.Start != "2026-09-14T14:02:00+09:00" || bus.Duration != 2280 || bus.DepartIn != 120 {
		t.Fatalf("출입구에서 걷는 버스 후보는 그대로: %s dur=%v wait=%v", bus.Start, bus.Duration, bus.DepartIn)
	}
	if bike.Start != "2026-09-14T14:00:00+09:00" || bike.End != "2026-09-14T14:30:00+09:00" || bike.Duration != 1800 ||
		bike.DepartIn != 0 || bike.Legs[0].Start != "2026-09-14T14:00:00+09:00" ||
		bike.Legs[1].End != "2026-09-14T14:30:00+09:00" {
		t.Fatalf("대중교통 없는 후보는 요청 시각에 출발하도록 2분 앞당긴다: %+v", bike)
	}
}

// 앵커링된 도착지라도 승강장에서 내리지 않는 후보(출입구까지 걷는 버스·도보)에는 이탈시간을 붙이지 않는다.
func TestStationExitOnlyForPlatformAlighting(t *testing.T) {
	f := &fakeOTP{}
	f.answer = func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{
			itin("2026-09-14T14:05:00+09:00", "2026-09-14T14:35:00+09:00",
				otp.Leg{Mode: "SUBWAY", Route: "4호선", TransitLeg: true, Start: "2026-09-14T14:05:00+09:00",
					End: "2026-09-14T14:35:00+09:00"}),
			itin("2026-09-14T14:05:00+09:00", "2026-09-14T14:38:00+09:00",
				otp.Leg{Mode: "BUS", Route: "401", TransitLeg: true, Start: "2026-09-14T14:05:00+09:00",
					End: "2026-09-14T14:35:00+09:00"},
				otp.Leg{Mode: "WALK", ToName: "서울 9-1번 출구", Start: "2026-09-14T14:35:00+09:00", End: "2026-09-14T14:38:00+09:00"}),
		}, nil
	}
	p := &Planner{OTP: f}
	p.SetStations([]otp.Station{seoulStation})
	dest := Point{Lat: 37.55407, Lon: 126.97070, Name: "서울역"}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: gangnam, Destination: dest})
	if err != nil || len(its) != 2 {
		t.Fatalf("its=%d err=%v", len(its), err)
	}
	for _, it := range its {
		last := it.Legs[len(it.Legs)-1]
		if last.Mode == "SUBWAY" && (it.End != "2026-09-14T14:36:00+09:00" || it.Duration != 1800+StationExitSec) {
			t.Fatalf("승강장에서 내리는 후보: 이탈 1분: %s dur=%v", it.End, it.Duration)
		}
		if last.Mode == "WALK" && (it.End != "2026-09-14T14:38:00+09:00" || it.Duration != 1980) {
			t.Fatalf("출입구까지 걷는 후보는 그대로: %s dur=%v", it.End, it.Duration)
		}
	}
}

// 도착 시각 요청은 요청 시각을 밀지 않으므로, 앵커링된 출발지의 대중교통 없는 후보도 시각을 되돌리지 않는다.
func TestArriveByKeepsWalkOnlyTimes(t *testing.T) {
	arrive := time.Date(2026, 10, 5, 9, 0, 0, 0, time.FixedZone("KST", 9*3600))
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-10-05T08:40:00+09:00", "2026-10-05T09:00:00+09:00",
			otp.Leg{Mode: "WALK", Start: "2026-10-05T08:40:00+09:00", End: "2026-10-05T09:00:00+09:00"})}, nil
	}}
	p := &Planner{OTP: f, Now: func() time.Time { return arrive.Add(-2 * time.Hour) }}
	p.SetStations([]otp.Station{seoulStation})
	origin := Point{Lat: 37.55407, Lon: 126.97070, Name: "서울역"}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: origin, Destination: gangnam, Arrive: &arrive})
	if err != nil || len(its) != 1 {
		t.Fatalf("its=%d err=%v", len(its), err)
	}
	if it := its[0]; it.Start != "2026-10-05T08:40:00+09:00" || it.End != "2026-10-05T09:00:00+09:00" ||
		it.Legs[0].Start != "2026-10-05T08:40:00+09:00" {
		t.Fatalf("도착 요청의 도보 후보는 그대로: %+v", it)
	}
}

// 앵커링된 도착지의 도착 시각 요청에서 승강장에서 내리지 않는 후보(버스·도보)는 지정 시각 그대로 찾고, 지정 시각에
// 도착하는 후보가 남는다. 승강장에서 내리는 후보는 −이탈시간 요청에서만 취한다.
func TestArriveByNonRailUsesExactLimit(t *testing.T) {
	arrive := time.Date(2026, 10, 5, 9, 0, 0, 0, time.FixedZone("KST", 9*3600))
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		end := r.Arrive.Format(time.RFC3339)
		start := r.Arrive.Add(-30 * time.Minute).Format(time.RFC3339)
		if r.Modes.TransitOnly && r.Modes.Transit.Modes[0].Mode == "SUBWAY" {
			return []otp.Itinerary{itin(start, end,
				otp.Leg{Mode: "SUBWAY", Route: "4호선", TransitLeg: true, Start: start, End: end})}, nil
		}
		if r.Modes.TransitOnly {
			return []otp.Itinerary{itin(start, end,
				otp.Leg{Mode: "BUS", Route: "401", TransitLeg: true, Start: start,
					End: r.Arrive.Add(-3 * time.Minute).Format(time.RFC3339)},
				otp.Leg{Mode: "WALK", Start: r.Arrive.Add(-3 * time.Minute).Format(time.RFC3339), End: end})}, nil
		}
		return []otp.Itinerary{
			itin(start, end, otp.Leg{Mode: "SUBWAY", Route: "4호선", TransitLeg: true, Start: start, End: end}),
			itin(start, end, otp.Leg{Mode: "WALK", Start: start, End: end}),
		}, nil
	}}
	p := &Planner{OTP: f, Now: func() time.Time { return arrive.Add(-2 * time.Hour) }}
	p.SetStations([]otp.Station{seoulStation})
	dest := Point{Lat: 37.55407, Lon: 126.97070, Name: "서울역"}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: gangnam, Destination: dest, Arrive: &arrive})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, it := range its {
		got[it.Legs[len(it.Legs)-1].Mode+"/"+it.Legs[0].Mode] = it.End
	}
	if got["WALK/WALK"] != "2026-10-05T09:00:00+09:00" || got["WALK/BUS"] != "2026-10-05T09:00:00+09:00" ||
		got["SUBWAY/SUBWAY"] != "2026-10-05T09:00:00+09:00" || len(its) != 3 {
		t.Fatalf("도보·버스 후보는 09:00 도착, 지하철 후보는 08:59 승강장 + 이탈 1분: %v", got)
	}
}
