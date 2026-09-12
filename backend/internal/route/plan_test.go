package route

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// fakeOTP 는 받은 요청을 기록하고, 요청별로 정해진 itinerary 를 돌려준다. viaBoth 가 두 goroutine 에서 부르므로 잠근다.
type fakeOTP struct {
	mu     sync.Mutex
	calls  []otp.Request
	answer func(otp.Request) ([]otp.Itinerary, error)
}

func (f *fakeOTP) Plan(_ context.Context, r otp.Request) ([]otp.Itinerary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r)
	return f.answer(r)
}

var (
	seoulStn = Point{Lat: 37.5547, Lon: 126.9707}
	yeouido  = Point{Lat: 37.5216, Lon: 126.9243}
	gangnam  = Point{Lat: 37.4979, Lon: 127.0276}
)

func itin(start, end string, legs ...otp.Leg) otp.Itinerary {
	s, _ := time.Parse(time.RFC3339, start)
	e, _ := time.Parse(time.RFC3339, end)
	return otp.Itinerary{Start: start, End: end, Duration: e.Sub(s).Seconds(), Legs: legs}
}

func TestPlanRejectsOutsideSeoul(t *testing.T) {
	p := &Planner{OTP: &fakeOTP{}}
	_, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: Point{Lat: 35.1, Lon: 129.0}})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("부산 좌표는 400 이어야 한다: %v", err)
	}
	_, err = p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Modes: []SegmentMode{"walk", "bike"}})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("segment_modes 길이 불일치는 400 이어야 한다: %v", err)
	}
}

// 경유지 없는 any 는 한 번만 호출하고 개인 속도를 주입한다.
func TestPlanSingleCallNoVia(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:40:00+09:00",
			otp.Leg{Mode: "BUS", Route: "402", FromName: "서울역", ToName: "강남"})}, nil
	}}
	p := &Planner{OTP: f}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam, WalkSpeed: 1.5})
	if err != nil || len(its) != 1 || len(f.calls) != 1 {
		t.Fatalf("its=%v err=%v calls=%d", its, err, len(f.calls))
	}
	c := f.calls[0]
	if c.WalkSpeed != 1.5 || c.BikeSpeed != DefaultBike || c.Modes.Transit == nil || c.First != DefaultFirst {
		t.Fatalf("요청 파라미터: %+v", c)
	}
}

// 경유지 있는 any: via 한 번 호출(대중교통) + 구간 분할 any 탐색을 합친다. 도보만으로 도는 후보가 살아남아야 한다.
func TestPlanViaMergesSingleAndSegmented(t *testing.T) {
	f := &fakeOTP{}
	f.answer = func(r otp.Request) ([]otp.Itinerary, error) {
		if len(r.Via) > 0 { // via 호출: 대중교통 41분
			return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:41:00+09:00",
				otp.Leg{Mode: "BUS", Route: "152", FromName: "A", ToName: "B"})}, nil
		}
		dep := time.Date(2026, 9, 14, 14, 0, 0, 0, time.FixedZone("KST", 9*3600))
		if r.Depart != nil {
			dep = *r.Depart
		}
		end := dep.Add(1 * time.Minute)
		return []otp.Itinerary{itin(dep.Format(time.RFC3339), end.Format(time.RFC3339),
			otp.Leg{Mode: "WALK", FromName: "x", ToName: "y"})}, nil
	}
	its, err := (&Planner{OTP: f}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Via: []Point{yeouido}})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 { // via 1회 + 구간 2개(각 후보 1개)
		t.Fatalf("호출 수 %d", len(f.calls))
	}
	if len(its) != 2 || its[0].Duration != 120 || its[0].Legs[0].Mode != "WALK" || its[1].Legs[0].Route != "152" {
		t.Fatalf("도보 2분 결합 경로가 1순위, 대중교통 41분이 2순위여야 한다: %+v", its)
	}
	if its[0].Transfers != 0 {
		t.Fatalf("도보→도보 경계는 환승이 아니다: %+v", its[0])
	}
}

// 한쪽만 경로 없음이면 다른 쪽 결과를 내고, 둘 다 없을 때만 ErrNoRoute.
func TestPlanViaOneSideNoRoute(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		if len(r.Via) > 0 {
			return nil, otp.ErrNoRoute
		}
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:10:00+09:00", otp.Leg{Mode: "WALK"})}, nil
	}}
	its, err := (&Planner{OTP: f}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Via: []Point{yeouido}})
	if err != nil || len(its) != 1 {
		t.Fatalf("its=%v err=%v", its, err)
	}
	g := &fakeOTP{answer: func(otp.Request) ([]otp.Itinerary, error) { return nil, otp.ErrNoRoute }}
	_, err = (&Planner{OTP: g}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Via: []Point{yeouido}})
	if !errors.Is(err, otp.ErrNoRoute) {
		t.Fatalf("둘 다 없으면 ErrNoRoute: %v", err)
	}
}

// 고정한 수단의 leg 가 없는 결과(OTP 가 도보로 때운 것)는 구간 단위로 걸러낸다.
func TestPlanLockedModeFiltersWalkOnlyResults(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{
			itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:02:00+09:00", otp.Leg{Mode: "WALK", ToName: "강남"}),
			itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:09:00+09:00",
				otp.Leg{Mode: "WALK"}, otp.Leg{Mode: "BICYCLE", RentedBike: true, ToName: "강남"}),
		}, nil
	}}
	its, err := (&Planner{OTP: f}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Modes: []SegmentMode{ModeBike}})
	if err != nil || len(its) != 1 || !its[0].Legs[1].RentedBike {
		t.Fatalf("bike 고정은 대여 자전거 leg 가 있는 후보만: %+v err=%v", its, err)
	}
	g := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		if !r.Modes.TransitOnly {
			return nil, errors.New("transit 고정은 transitOnly 로 보내야 한다")
		}
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:02:00+09:00", otp.Leg{Mode: "WALK"})}, nil
	}}
	_, err = (&Planner{OTP: g}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Modes: []SegmentMode{ModeTransit}})
	if !errors.Is(err, otp.ErrNoRoute) {
		t.Fatalf("transit 고정에 대중교통 leg 없는 결과만 오면 경로 없음: %v", err)
	}
}

// frequencies 기반 중복(출발시각만 다른 같은 leg 열)은 가장 이른 것 하나만 남긴다.
func TestPlanDedupesSameLegSignature(t *testing.T) {
	bus := otp.Leg{Mode: "BUS", Route: "402", FromName: "서울역", ToName: "강남역"}
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{
			itin("2026-09-14T14:02:00+09:00", "2026-09-14T14:41:00+09:00", bus),
			itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:39:00+09:00", bus),
			itin("2026-09-14T14:01:00+09:00", "2026-09-14T14:40:00+09:00", bus),
			itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:50:00+09:00",
				otp.Leg{Mode: "SUBWAY", Route: "4", FromName: "서울역", ToName: "사당"}),
		}, nil
	}}
	its, err := (&Planner{OTP: f}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil {
		t.Fatal(err)
	}
	if len(its) != 2 || its[0].Start != "2026-09-14T14:00:00+09:00" || its[0].Legs[0].Route != "402" {
		t.Fatalf("중복 제거·정렬 결과: %+v", its)
	}
}

// 구간별 수단 고정: 구간마다 호출하고 앞 구간 도착시각을 다음 구간 출발로 넘긴다.
func TestPlanSegmentedBeam(t *testing.T) {
	f := &fakeOTP{}
	f.answer = func(r otp.Request) ([]otp.Itinerary, error) {
		if len(f.calls) == 1 { // 1구간(walk): 후보 2개
			return []otp.Itinerary{
				itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:30:00+09:00", otp.Leg{Mode: "WALK", ToName: "여의도"}),
				itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:25:00+09:00", otp.Leg{Mode: "WALK", ToName: "여의도B"}),
			}, nil
		}
		dep := r.Depart.Format(time.RFC3339)
		end := r.Depart.Add(20 * time.Minute).Format(time.RFC3339)
		return []otp.Itinerary{itin(dep, end, otp.Leg{Mode: "BICYCLE", RentedBike: true, FromName: "여의도", ToName: "강남"})}, nil
	}
	its, err := (&Planner{OTP: f}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Via: []Point{yeouido}, Modes: []SegmentMode{ModeWalk, ModeBike}})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 { // 1 + 후보 2개 × 2구간
		t.Fatalf("호출 수 %d: %+v", len(f.calls), f.calls)
	}
	if !f.calls[0].Modes.Only || f.calls[0].Modes.Direct[0] != "WALK" || f.calls[1].Modes.Direct[0] != "BICYCLE_RENTAL" {
		t.Fatalf("구간 모드 매핑: %+v %+v", f.calls[0].Modes, f.calls[1].Modes)
	}
	if f.calls[1].Depart == nil || f.calls[1].Depart.Format(time.RFC3339) != "2026-09-14T14:25:00+09:00" {
		t.Fatalf("2구간 출발은 가장 이른 1구간 도착(14:25)이어야 한다: %v", f.calls[1].Depart)
	}
	best := its[0]
	if best.End != "2026-09-14T14:45:00+09:00" || len(best.Legs) != 2 || best.Duration != 45*60 || best.Transfers != 1 {
		t.Fatalf("결합 결과(walk→bike 경계는 환승 1): %+v", best)
	}
}

// 구간 경계 환승 규칙: walk,walk → 0 / transit,transit → 1 (버스에서 내려 다른 버스).
func TestSegmentBoundaryTransfers(t *testing.T) {
	f := &fakeOTP{}
	f.answer = func(r otp.Request) ([]otp.Itinerary, error) {
		dep := time.Date(2026, 9, 14, 14, 0, 0, 0, time.FixedZone("KST", 9*3600))
		if r.Depart != nil {
			dep = *r.Depart
		}
		end := dep.Add(10 * time.Minute)
		legs := []otp.Leg{{Mode: "WALK"}}
		if r.Modes.TransitOnly {
			legs = []otp.Leg{{Mode: "WALK"}, {Mode: "BUS", Route: "1", TransitLeg: true}, {Mode: "WALK"}}
		}
		return []otp.Itinerary{itin(dep.Format(time.RFC3339), end.Format(time.RFC3339), legs...)}, nil
	}
	its, err := (&Planner{OTP: f}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Via: []Point{yeouido}, Modes: []SegmentMode{ModeWalk, ModeWalk}})
	if err != nil || its[0].Transfers != 0 {
		t.Fatalf("walk,walk transfers=%d err=%v", its[0].Transfers, err)
	}
	its, err = (&Planner{OTP: f}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Via: []Point{yeouido}, Modes: []SegmentMode{ModeTransit, ModeTransit}})
	if err != nil || its[0].Transfers != 1 {
		t.Fatalf("transit,transit transfers=%d err=%v", its[0].Transfers, err)
	}
}

func TestPlanNoRoutePropagates(t *testing.T) {
	f := &fakeOTP{answer: func(otp.Request) ([]otp.Itinerary, error) { return nil, otp.ErrNoRoute }}
	_, err := (&Planner{OTP: f}).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if !errors.Is(err, otp.ErrNoRoute) {
		t.Fatalf("err=%v", err)
	}
}
