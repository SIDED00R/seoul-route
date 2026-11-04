package route

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 열차 안 출발지는 이름이 "현재 위치" 여도 300m 안의 가장 가까운 부모역으로 앵커링하고, 반경 밖이면 앵커링하지 않는다.
func TestAnchorOnBoardNearestStation(t *testing.T) {
	p := &Planner{}
	p.SetStations([]otp.Station{seoulStation, {ID: "seoul:ST_강남", Name: "강남", Lat: 37.4979, Lon: 127.0276}})
	near := Point{Lat: 37.5550, Lon: 126.9721, Name: "현재 위치", OnBoard: true} // 서울역에서 북쪽 약 130m
	if got := p.anchor(near); got != "seoul:ST_서울" {
		t.Fatalf("300m 안 가장 가까운 역이어야: %q", got)
	}
	far := Point{Lat: 37.5580, Lon: 126.9721, Name: "현재 위치", OnBoard: true} // 약 470m
	if got := p.anchor(far); got != "" {
		t.Fatalf("300m 밖이면 앵커링 안 함: %q", got)
	}
	near.OnBoard = false
	if got := p.anchor(near); got != "" {
		t.Fatalf("열차 안이 아니면 이름 규칙 그대로(현재 위치는 좌표): %q", got)
	}
}

// 배선: 열차 안 출발지는 역 ID 로 요청하되 요청 시각을 진입시간만큼 밀지 않고, 여정 출발도 앞당기지 않는다.
// 구간 수단을 고정한 요청(구간별 탐색)도 같다.
func TestPlanOnBoardSkipsStationEntry(t *testing.T) {
	fixed := time.Date(2026, 11, 4, 14, 0, 0, 0, time.FixedZone("KST", 9*3600))
	f := &fakeOTP{}
	f.answer = func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-11-04T14:01:00+09:00", "2026-11-04T14:21:00+09:00",
			otp.Leg{Mode: "SUBWAY", Route: "5호선", TransitLeg: true,
				Start: "2026-11-04T14:01:00+09:00", End: "2026-11-04T14:21:00+09:00"})}, nil
	}
	p := &Planner{OTP: f, Now: func() time.Time { return fixed }}
	p.SetStations([]otp.Station{seoulStation})
	origin := Point{Lat: 37.5550, Lon: 126.9721, Name: "현재 위치", OnBoard: true}
	ctx := context.Background()

	for _, modes := range [][]SegmentMode{nil, {ModeTransit}} {
		f.calls = nil
		its, err := p.Plan(ctx, PlanRequest{Origin: origin, Destination: gangnam, Modes: modes})
		if err != nil || len(its) == 0 {
			t.Fatalf("modes=%v its=%d err=%v", modes, len(its), err)
		}
		for _, c := range f.calls {
			if c.OriginStop != "seoul:ST_서울" || c.Depart != nil {
				t.Fatalf("modes=%v 역 ID 로, 요청 시각은 그대로(nil): stop=%q depart=%v", modes, c.OriginStop, c.Depart)
			}
		}
		if it := its[0]; it.Start != "2026-11-04T14:01:00+09:00" || it.Duration != 1200 || it.DepartIn != 60 {
			t.Fatalf("modes=%v 진입시간 없음: start=%s dur=%v wait=%v", modes, it.Start, it.Duration, it.DepartIn)
		}
	}
}

// 열차 안 출발지 근처에 역이 없으면 OTP 를 부르지 않고 경로 없음이다(좌표로 찾으면 지상을 걷는 경로가 나온다).
func TestPlanOnBoardWithoutStationIsNoRoute(t *testing.T) {
	f := &fakeOTP{answer: func(otp.Request) ([]otp.Itinerary, error) { return nil, nil }}
	p := &Planner{OTP: f}
	p.SetStations([]otp.Station{seoulStation})
	origin := Point{Lat: 37.5580, Lon: 126.9721, Name: "현재 위치", OnBoard: true}
	_, err := p.Plan(context.Background(), PlanRequest{Origin: origin, Destination: gangnam})
	if !errors.Is(err, otp.ErrNoRoute) || len(f.calls) != 0 {
		t.Fatalf("경로 없음·OTP 호출 0 이어야: err=%v calls=%d", err, len(f.calls))
	}
}
