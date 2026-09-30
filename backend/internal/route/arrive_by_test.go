package route

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

func TestValidateArrive(t *testing.T) {
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, time.FixedZone("KST", 9*3600))
	later, past := now.Add(time.Hour), now.Add(-time.Minute)
	any1 := []SegmentMode{ModeAny}
	cases := []struct {
		name string
		req  PlanRequest
		ok   bool
	}{
		{"도착만", PlanRequest{Arrive: &later, Modes: any1}, true},
		{"도착 없음", PlanRequest{Depart: &past, Modes: any1}, true},
		{"출발과 함께", PlanRequest{Arrive: &later, Depart: &later, Modes: any1}, false},
		{"경유지", PlanRequest{Arrive: &later, Via: []Point{yeouido}, Modes: []SegmentMode{ModeAny, ModeAny}}, false},
		{"수단 고정", PlanRequest{Arrive: &later, Modes: []SegmentMode{ModeWalk}}, false},
		{"지난 시각", PlanRequest{Arrive: &past, Modes: any1}, false},
	}
	for _, c := range cases {
		err := validateArrive(c.req, now)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrBadRequest)) {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
}

// 배선: 도착 시각 요청은 OTP 에 latestArrival 로 가고(도착지가 역이면 이탈 1분 앞당김) 출발 시각은 비운다. 지정 시각보다
// 늦게 도착하는 여정은 빠지고(정각 도착은 남는다), 같은 구간 열이면 늦게 출발하는 여정이 남는다. 실시간 보정은 하지 않는다.
func TestPlanArriveBy(t *testing.T) {
	arrive, _ := time.Parse(time.RFC3339, "2026-10-05T09:00:00+09:00")
	// 역에서 바로 타고 내리는 지하철(진입·이탈 가산 대상)
	rail := func(route string) otp.Leg {
		return otp.Leg{Mode: "SUBWAY", Route: route, FromName: "서울역", ToName: "강남", TransitLeg: true}
	}
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{
			itin("2026-10-05T08:20:00+09:00", "2026-10-05T08:50:00+09:00", rail("402")),
			itin("2026-10-05T08:30:00+09:00", "2026-10-05T08:59:00+09:00", rail("402")),
			itin("2026-10-05T08:35:00+09:00", "2026-10-05T09:05:00+09:00", rail("740")),
		}, nil
	}}
	rt := &fakeRealtime{}
	p := &Planner{OTP: f, Realtime: rt, Now: func() time.Time { return arrive.Add(-2 * time.Hour) }}
	p.SetStations([]otp.Station{{ID: "seoul:ST_강남", Name: "강남", Lat: gangnam.Lat, Lon: gangnam.Lon},
		{ID: "seoul:ST_서울", Name: "서울", Lat: seoulStn.Lat, Lon: seoulStn.Lon}})
	origin, dest := seoulStn, gangnam
	origin.Name, dest.Name = "서울역", "강남역"
	its, err := p.Plan(context.Background(), PlanRequest{Origin: origin, Destination: dest, Arrive: &arrive})
	if err != nil {
		t.Fatal(err)
	}
	// 변형 4개: 전체 수단은 지정 시각과 지정 −1분 두 번, 지하철 전용은 −1분, 버스 전용은 지정 시각.
	exact, early := 0, 0
	for _, c := range f.calls {
		if c.Depart != nil || c.Arrive == nil || c.DestStop != "seoul:ST_강남" || c.OriginStop != "seoul:ST_서울" {
			t.Fatalf("OTP 요청: depart=%v arrive=%v origin=%q dest=%q", c.Depart, c.Arrive, c.OriginStop, c.DestStop)
		}
		switch {
		case c.Arrive.Equal(arrive):
			exact++
		case c.Arrive.Equal(arrive.Add(-time.Minute)):
			early++
		default:
			t.Fatalf("도착 제한: %v", c.Arrive)
		}
	}
	if len(f.calls) != 4 || exact != 2 || early != 2 {
		t.Fatalf("변형 요청 수: 전체 %d, 지정 시각 %d, −1분 %d", len(f.calls), exact, early)
	}
	// 402 는 08:30 출발(도착 08:59 + 이탈 1분 = 09:00)만 남고, 740(09:05 + 1분)은 빠진다. 출발은 역 진입 2분 앞당김.
	if len(its) != 1 || its[0].Start != "2026-10-05T08:28:00+09:00" || its[0].End != "2026-10-05T09:00:00+09:00" ||
		its[0].DepartIn != 0 {
		t.Fatalf("its=%+v", its)
	}
	if rt.calls != 0 {
		t.Errorf("도착 시각 요청에 실시간 보정 %d회", rt.calls)
	}

	// 전부 늦으면 경로 없음
	late := arrive.Add(-30 * time.Minute)
	if _, err := p.Plan(context.Background(), PlanRequest{Origin: origin, Destination: dest, Arrive: &late}); !errors.Is(
		err, otp.ErrNoRoute) {
		t.Errorf("err=%v", err)
	}
}

// 같은 구간 열의 늦은 여정이 횡단보도 대기로 지정 시각을 넘거나(마지막 도보) 환승을 놓치면(환승 도보), 그 여정만
// 빠지고 이른 여정은 남는다 — 중복 정리는 대기를 더하고 지정 시각으로 거른 뒤에 한다.
func TestPlanArriveByKeepsEarlierWhenLaterFails(t *testing.T) {
	arrive, _ := time.Parse(time.RFC3339, "2026-10-05T09:00:00+09:00")
	walk := func(poly, s, e string) otp.Leg { return otp.Leg{Mode: "WALK", Polyline: poly, Start: s, End: e} }
	ride := func(route, s, e string) otp.Leg {
		return otp.Leg{Mode: "BUS", Route: route, FromName: "A", ToName: "B", TransitLeg: true, Start: s, End: e}
	}
	at := func(hm string) string { return "2026-10-05T" + hm + ":00+09:00" }
	cases := []struct {
		name         string
		early, later otp.Itinerary
	}{
		{"마지막 도보",
			itin(at("08:20"), at("08:52"), ride("402", at("08:20"), at("08:45")), walk("w", at("08:45"), at("08:52"))),
			itin(at("08:30"), at("08:59"), ride("402", at("08:30"), at("08:52")), walk("w", at("08:52"), at("08:59")))},
		{"환승 도보",
			itin(at("08:10"), at("08:50"), ride("402", at("08:10"), at("08:25")), walk("w", at("08:25"), at("08:27")),
				ride("740", at("08:35"), at("08:50"))),
			itin(at("08:20"), at("08:58"), ride("402", at("08:20"), at("08:35")), walk("w", at("08:35"), at("08:37")),
				ride("740", at("08:38"), at("08:58")))},
	}
	for _, c := range cases {
		f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
			if r.Modes.Transit != nil && len(r.Modes.Transit.Modes) > 0 {
				return nil, otp.ErrNoRoute
			}
			return []otp.Itinerary{c.later, c.early}, nil
		}}
		p := &Planner{OTP: f, Crossings: fakeCrossings{"w": 4}, CrossingSec: 30,
			Now: func() time.Time { return arrive.Add(-2 * time.Hour) }}
		its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam, Arrive: &arrive})
		if err != nil || len(its) != 1 || its[0].Start != c.early.Start {
			t.Errorf("%s: its=%+v err=%v", c.name, its, err)
		}
	}
}

// 배선: 도착 시각 요청의 도보만 여정은 횡단보도 대기만큼 일찍 출발해 지정 시각에 도착한다(대기로 늦춰져 빠지지 않는다).
func TestPlanArriveByCrossings(t *testing.T) {
	arrive, _ := time.Parse(time.RFC3339, "2026-10-05T09:00:00+09:00")
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		if r.Modes.Transit != nil && len(r.Modes.Transit.Modes) > 0 {
			return nil, otp.ErrNoRoute
		}
		return []otp.Itinerary{itin("2026-10-05T08:40:00+09:00", "2026-10-05T09:00:00+09:00",
			otp.Leg{Mode: "WALK", Polyline: "w", Duration: 1200,
				Start: "2026-10-05T08:40:00+09:00", End: "2026-10-05T09:00:00+09:00"})}, nil
	}}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"w": 2}, CrossingSec: 30,
		Now: func() time.Time { return arrive.Add(-time.Hour) }}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam, Arrive: &arrive})
	if err != nil || len(its) != 1 || its[0].Start != "2026-10-05T08:39:00+09:00" ||
		its[0].End != "2026-10-05T09:00:00+09:00" {
		t.Fatalf("its=%+v err=%v", its, err)
	}
}

// 배선: 도착 시각 요청은 따릉이 한도를 넘는 대여를 나누지 않고 뺀다(나누기는 출발 시각에서 앞으로 다시 탐색한다).
func TestPlanArriveByDropsLongRental(t *testing.T) {
	arrive := time.Date(2026, 9, 14, 16, 0, 0, 0, kst)
	f := splitOTP(70)
	p := &Planner{OTP: f, Bikes: splitBikes(), Now: func() time.Time { return arrive.Add(-3 * time.Hour) }}
	_, err := p.Plan(context.Background(),
		PlanRequest{Origin: rentFrom, Destination: rentTo, BikeLimitMin: 60, Arrive: &arrive})
	if !errors.Is(err, otp.ErrNoRoute) {
		t.Fatalf("err=%v", err)
	}
	for _, c := range f.calls {
		if c.Arrive == nil {
			t.Fatalf("도착 시각 없이 다시 탐색했다: %+v", c)
		}
	}
}

// 횡단보도 대기: 도보만 여정은 대기만큼 일찍 출발하고 도착은 그대로, 첫 탑승을 놓치면 넘친 만큼 일찍 출발, 대중교통을 탄
// 뒤 환승 도보에서 놓치면 뺀다.
func TestArriveCrossings(t *testing.T) {
	cx := fakeCrossings{"w": 4}
	walk := func(poly, s, e string) otp.Leg { return otp.Leg{Mode: "WALK", Polyline: poly, Start: s, End: e} }
	ride := func(s, e string) otp.Leg { return otp.Leg{Mode: "BUS", TransitLeg: true, Start: s, End: e} }
	run := func(it otp.Itinerary) (otp.Itinerary, bool) {
		end0 := it.End
		k, acc := addCrossingWaits(&it, cx, 30)
		ok := arriveCrossings(&it, k, acc, end0)
		return it, ok
	}

	it, ok := run(itin("2026-10-05T08:40:00+09:00", "2026-10-05T09:00:00+09:00",
		walk("w", "2026-10-05T08:40:00+09:00", "2026-10-05T09:00:00+09:00")))
	if !ok || it.Start != "2026-10-05T08:38:00+09:00" || it.End != "2026-10-05T09:00:00+09:00" || it.Duration != 1320 {
		t.Errorf("도보만: %+v", it)
	}

	// 대기 120초, 탑승까지 여유 60초 → 60초 일찍
	it, ok = run(itin("2026-10-05T08:30:00+09:00", "2026-10-05T08:58:00+09:00",
		walk("w", "2026-10-05T08:30:00+09:00", "2026-10-05T08:40:00+09:00"),
		ride("2026-10-05T08:41:00+09:00", "2026-10-05T08:55:00+09:00"),
		walk("", "2026-10-05T08:55:00+09:00", "2026-10-05T08:58:00+09:00")))
	if !ok || it.Start != "2026-10-05T08:29:00+09:00" || it.End != "2026-10-05T08:58:00+09:00" {
		t.Errorf("첫 탑승: %+v", it)
	}

	_, ok = run(itin("2026-10-05T08:20:00+09:00", "2026-10-05T08:55:00+09:00",
		walk("", "2026-10-05T08:20:00+09:00", "2026-10-05T08:25:00+09:00"),
		ride("2026-10-05T08:26:00+09:00", "2026-10-05T08:40:00+09:00"),
		walk("w", "2026-10-05T08:40:00+09:00", "2026-10-05T08:42:00+09:00"),
		ride("2026-10-05T08:43:00+09:00", "2026-10-05T08:55:00+09:00")))
	if ok {
		t.Error("환승 도보에서 놓친 여정이 남았다")
	}
}
