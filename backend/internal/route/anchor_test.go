package route

import (
	"context"
	"testing"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

var seoulStation = otp.Station{ID: "seoul:ST_서울", Name: "서울", Lat: 37.5538, Lon: 126.9721}

func TestStationBase(t *testing.T) {
	cases := map[string]string{
		"서울역": "서울", "강남역 2호선": "강남", "서울역 공항철도": "서울", "여의도역 5호선": "여의도",
		"스타벅스 서울역점": "", "역": "", "": "", "롯데마트 제타플렉스 서울역점": "", "서울특별시청": "",
	}
	for in, want := range cases {
		if got := stationBase(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

// 카카오 "서울역"(역사 건물 좌표)은 100m 옆 부모역으로 앵커링되고, 이름이 다르거나 1km 밖이면 좌표 그대로.
func TestAnchor(t *testing.T) {
	p := &Planner{}
	p.SetStations([]otp.Station{seoulStation, {ID: "seoul:ST_강남", Name: "강남", Lat: 37.4979, Lon: 127.0276}})
	if got := p.anchor(Point{Lat: 37.55407, Lon: 126.97070, Name: "서울역"}); got != "seoul:ST_서울" {
		t.Fatalf("서울역 → ST_서울 이어야: %q", got)
	}
	if got := p.anchor(Point{Lat: 37.55407, Lon: 126.97070, Name: "서울역 공항철도"}); got != "seoul:ST_서울" {
		t.Fatalf("노선 접미사는 무시: %q", got)
	}
	if got := p.anchor(Point{Lat: 37.55407, Lon: 126.97070, Name: "강남역 2호선"}); got != "" {
		t.Fatalf("이름은 맞아도 1km 밖이면 앵커링 안 함: %q", got)
	}
	if got := p.anchor(Point{Lat: 37.55407, Lon: 126.97070, Name: "카페 서울역점"}); got != "" {
		t.Fatalf("역이 아닌 장소는 좌표: %q", got)
	}
	if got := (&Planner{}).anchor(Point{Name: "서울역"}); got != "" {
		t.Fatalf("역 목록이 없으면 좌표: %q", got)
	}
}

// 배선: /routes/plan 요청의 name 이 OTP 요청의 OriginStop/DestStop 으로 넘어간다. 도보 전용 구간은 좌표 그대로.
func TestPlanPassesAnchorsToOTP(t *testing.T) {
	f := &fakeOTP{}
	f.answer = func(r otp.Request) ([]otp.Itinerary, error) {
		if r.Modes.Only { // 도보 전용 호출
			return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T15:30:00+09:00", otp.Leg{Mode: "WALK"})}, nil
		}
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:30:00+09:00",
			otp.Leg{Mode: "WALK"}, otp.Leg{Mode: "BUS", Route: "402", TransitLeg: true}, otp.Leg{Mode: "WALK"})}, nil
	}
	p := &Planner{OTP: f}
	p.SetStations([]otp.Station{seoulStation})
	origin := Point{Lat: 37.55407, Lon: 126.97070, Name: "서울역"}
	if _, err := p.Plan(context.Background(), PlanRequest{Origin: origin, Destination: gangnam}); err != nil {
		t.Fatal(err)
	}
	if f.calls[0].OriginStop != "seoul:ST_서울" || f.calls[0].DestStop != "" {
		t.Fatalf("단일 호출 앵커: %+v", f.calls[0])
	}
	// 경유 역도 역 ID 로 넘긴다(via 호출 3회 중 어느 것이든 ViaStops 가 채워져야 한다).
	f.calls = nil
	viaReq := PlanRequest{Origin: gangnam, Destination: yeouido,
		Via: []Point{{Lat: 37.55407, Lon: 126.97070, Name: "서울역"}}}
	if _, err := p.Plan(context.Background(), viaReq); err != nil {
		t.Fatal(err)
	}
	viaAnchored := 0
	for _, c := range f.calls {
		if len(c.Via) == 1 && len(c.ViaStops) == 1 && c.ViaStops[0] == "seoul:ST_서울" {
			viaAnchored++
		}
	}
	if viaAnchored != 3 {
		t.Fatalf("via 호출 3회 모두 경유 역 ID 를 실어야: %d / %+v", viaAnchored, f.calls)
	}
	f.calls = nil
	_, err := p.Plan(context.Background(), PlanRequest{Origin: origin, Destination: gangnam,
		Modes: []SegmentMode{ModeWalk}})
	if err != nil {
		t.Fatal(err)
	}
	if f.calls[0].OriginStop != "" {
		t.Fatalf("도보 전용 구간은 좌표로 요청해야: %+v", f.calls[0])
	}
}
