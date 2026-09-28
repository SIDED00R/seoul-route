package route

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/gbfs"
	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// encodePolyline 은 Google encoded polyline(정밀도 1e-5) 부호기. crossing.DecodePolyline 의 역이다.
func encodePolyline(pts [][2]float64) string {
	var b strings.Builder
	put := func(v int) {
		u := v << 1
		if v < 0 {
			u = ^u
		}
		for u >= 0x20 {
			b.WriteByte(byte((0x20 | (u & 0x1f)) + 63))
			u >>= 5
		}
		b.WriteByte(byte(u + 63))
	}
	pLat, pLon := 0, 0
	for _, p := range pts {
		lat, lon := int(math.Round(p[0]*1e5)), int(math.Round(p[1]*1e5))
		put(lat - pLat)
		put(lon - pLon)
		pLat, pLon = lat, lon
	}
	return b.String()
}

// 위도 37.5 에서 경도 126.90 → 127.10 으로 곧게 가는 경로선(0.01° 간격 21점).
func eastLine() string {
	var pts [][2]float64
	for i := 0; i <= 20; i++ {
		pts = append(pts, [2]float64{37.5, 126.90 + 0.01*float64(i)})
	}
	return encodePolyline(pts)
}

var (
	kst       = time.FixedZone("KST", 9*3600)
	rentFrom  = Point{Lat: 37.5, Lon: 126.899}
	rentTo    = Point{Lat: 37.5, Lon: 127.101}
	stationS2 = gbfs.Station{ID: "S2", Name: "S2", Lat: 37.5005, Lon: 127.04} // 경로의 70% 지점
)

// 대여소: 30%·70%·90% 지점 옆(50m 남짓). 60분권 예산 55분 / 대여 70분 = 78.6% 안에서 가장 먼 곳은 70%(S2).
func splitBikes() fakeBikes {
	return fakeBikes{snap: &gbfs.Snapshot{Stations: []gbfs.Station{
		{ID: "S1", Name: "S1", Lat: 37.5005, Lon: 126.96},
		stationS2,
		{ID: "S3", Name: "S3", Lat: 37.5005, Lon: 127.08},
	}}}
}

func bikeTrip(start time.Time, rideMin int, poly string) otp.Itinerary {
	pick := start.Add(5 * time.Minute)
	drop := pick.Add(time.Duration(rideMin) * time.Minute)
	end := drop.Add(3 * time.Minute)
	f := func(t time.Time) string { return t.Format(time.RFC3339) }
	return itin(f(start), f(end),
		otp.Leg{Mode: "WALK", Duration: 300, Start: f(start), End: f(pick)},
		otp.Leg{Mode: "BICYCLE", RentedBike: true, Duration: float64(rideMin * 60), Polyline: poly,
			Start: f(pick), End: f(drop), FromName: "A", ToName: "B"},
		otp.Leg{Mode: "WALK", Duration: 180, Start: f(drop), End: f(end)})
}

func isBikeOnly(r otp.Request) bool {
	return r.Modes.Only && len(r.Modes.Direct) > 0 && r.Modes.Direct[0] == "BICYCLE_RENTAL"
}

// 따릉이만 70분 가는 후보 하나. 전체 탐색에만 나오고 지하철만·버스만 탐색은 경로 없음.
// 대여소 S2 까지(앞쪽) 45분 대여, S2 부터(뒤쪽) 25분 대여로 다시 탐색된다.
func splitOTP(ride int) *fakeOTP {
	start := time.Date(2026, 9, 14, 14, 0, 0, 0, kst)
	return &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		switch {
		case isBikeOnly(r) && r.Destination == coord(Point{Lat: stationS2.Lat, Lon: stationS2.Lon}):
			return []otp.Itinerary{bikeTrip(start, 45, "")}, nil
		case isBikeOnly(r) && r.Origin == coord(Point{Lat: stationS2.Lat, Lon: stationS2.Lon}):
			return []otp.Itinerary{bikeTrip(*r.Depart, 25, "")}, nil
		case r.Modes.Transit != nil && len(r.Modes.Transit.Modes) > 0:
			return nil, otp.ErrNoRoute
		}
		return []otp.Itinerary{bikeTrip(start, ride, eastLine())}, nil
	}}
}

func rentedMinutes(it otp.Itinerary) []int {
	var out []int
	for _, l := range it.Legs {
		if l.RentedBike {
			out = append(out, int(l.Duration/60))
		}
	}
	return out
}

// 1시간권: 70분 대여는 S2 에서 반납하고 다시 빌리는 45분 + 25분 대여로 바뀐다(환승 0).
func TestPlanSplitsLongRentalAtStation(t *testing.T) {
	f := splitOTP(70)
	its, err := (&Planner{OTP: f, Bikes: splitBikes()}).Plan(context.Background(),
		PlanRequest{Origin: rentFrom, Destination: rentTo, BikeLimitMin: 60})
	if err != nil {
		t.Fatal(err)
	}
	if len(its) != 1 {
		t.Fatalf("나눈 후보 하나만 남아야: %+v", its)
	}
	got := rentedMinutes(its[0])
	if len(got) != 2 || got[0] != 45 || got[1] != 25 || its[0].Transfers != 0 {
		t.Fatalf("45분 + 25분 대여, 환승 0: %v %+v", got, its[0])
	}
	var head, tail *otp.Request
	for i := range f.calls {
		if isBikeOnly(f.calls[i]) && f.calls[i].Destination.Lon == stationS2.Lon {
			head = &f.calls[i]
		}
		if isBikeOnly(f.calls[i]) && f.calls[i].Origin.Lon == stationS2.Lon {
			tail = &f.calls[i]
		}
	}
	if head == nil || tail == nil || head.Origin != coord(rentFrom) || tail.Destination != coord(rentTo) {
		t.Fatalf("출발→S2, S2→도착 을 따릉이로 다시 탐색해야: %+v", f.calls)
	}
	if !tail.Depart.Equal(time.Date(2026, 9, 14, 14, 53, 0, 0, kst)) {
		t.Fatalf("뒤쪽은 앞쪽 도착(14:53)에 출발: %v", tail.Depart)
	}
}

// 고른 대여소(S2)까지 다시 탐색한 대여가 한도를 넘으면 SplitSlackSec 더 앞에서 고른 대여소(55% 지점)로 다시 나눈다.
func TestPlanRetriesEarlierStationWhenHeadTooLong(t *testing.T) {
	mid := gbfs.Station{ID: "M", Name: "M", Lat: 37.5005, Lon: 127.01}
	bikes := fakeBikes{snap: &gbfs.Snapshot{Stations: []gbfs.Station{stationS2, mid}}}
	start := time.Date(2026, 9, 14, 14, 0, 0, 0, kst)
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		switch {
		case isBikeOnly(r) && r.Destination.Lon == stationS2.Lon:
			return []otp.Itinerary{bikeTrip(start, 60, "")}, nil // 한도(55분) 초과
		case isBikeOnly(r) && r.Destination.Lon == mid.Lon:
			return []otp.Itinerary{bikeTrip(start, 40, "")}, nil
		case isBikeOnly(r) && r.Origin.Lon == mid.Lon:
			return []otp.Itinerary{bikeTrip(*r.Depart, 30, "")}, nil
		case r.Modes.Transit != nil && len(r.Modes.Transit.Modes) > 0:
			return nil, otp.ErrNoRoute
		}
		return []otp.Itinerary{bikeTrip(start, 70, eastLine())}, nil
	}}
	its, err := (&Planner{OTP: f, Bikes: bikes}).Plan(context.Background(),
		PlanRequest{Origin: rentFrom, Destination: rentTo, BikeLimitMin: 60})
	if err != nil {
		t.Fatal(err)
	}
	if got := rentedMinutes(its[0]); len(its) != 1 || len(got) != 2 || got[0] != 40 || got[1] != 30 {
		t.Fatalf("S2 실패 뒤 55%% 지점 대여소로 40분 + 30분: %+v", its)
	}
}

// 반납 대여소부터 도착까지 OTP 가 도보만 돌려주면(남은 거리가 짧을 때) 한도 안 대여 + 반납 뒤 도보로 잇는다.
func TestPlanSplitKeepsWalkOnlyTail(t *testing.T) {
	start := time.Date(2026, 9, 14, 14, 0, 0, 0, kst)
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		switch {
		case isBikeOnly(r) && r.Destination.Lon == stationS2.Lon:
			return []otp.Itinerary{bikeTrip(start, 45, "")}, nil
		case isBikeOnly(r) && r.Origin.Lon == stationS2.Lon:
			end := r.Depart.Add(8 * time.Minute)
			return []otp.Itinerary{itin(r.Depart.Format(time.RFC3339), end.Format(time.RFC3339),
				otp.Leg{Mode: "WALK", Duration: 480})}, nil
		case r.Modes.Transit != nil && len(r.Modes.Transit.Modes) > 0:
			return nil, otp.ErrNoRoute
		}
		return []otp.Itinerary{bikeTrip(start, 70, eastLine())}, nil
	}}
	its, err := (&Planner{OTP: f, Bikes: splitBikes()}).Plan(context.Background(),
		PlanRequest{Origin: rentFrom, Destination: rentTo, BikeLimitMin: 60})
	if err != nil {
		t.Fatal(err)
	}
	legs := its[0].Legs
	if got := rentedMinutes(its[0]); len(its) != 1 || len(got) != 1 || got[0] != 45 || legs[len(legs)-1].Mode != "WALK" {
		t.Fatalf("45분 대여 뒤 도보 8분으로 끝나야: %+v", its)
	}
}

// 2시간권이거나 한도를 보내지 않으면(0) 70분 대여를 그대로 둔다.
func TestPlanKeepsRentalWithinLimit(t *testing.T) {
	for _, limit := range []int{0, 120} {
		its, err := (&Planner{OTP: splitOTP(70), Bikes: splitBikes()}).Plan(context.Background(),
			PlanRequest{Origin: rentFrom, Destination: rentTo, BikeLimitMin: limit})
		if err != nil {
			t.Fatal(err)
		}
		if got := rentedMinutes(its[0]); len(its) != 1 || len(got) != 1 || got[0] != 70 {
			t.Fatalf("한도 %d: 70분 대여 그대로: %+v", limit, its)
		}
	}
}

// 대여 54분은 예산(55분) 안이지만 경로선의 신호 횡단보도 5곳 × 38초를 더하면 넘는다.
func TestRentalLimitCountsCrossingWaits(t *testing.T) {
	p := &Planner{CrossingSec: 38}
	legs := bikeTrip(time.Date(2026, 9, 14, 14, 0, 0, 0, kst), 54, "poly").Legs
	if _, over := p.overLimit(legs, rentalLimitSec(PlanRequest{BikeLimitMin: 60})); over {
		t.Fatal("횡단보도 없이 54분은 한도 안")
	}
	p.Crossings = fakeCrossings{"poly": 5}
	if _, over := p.overLimit(legs, rentalLimitSec(PlanRequest{BikeLimitMin: 60})); !over {
		t.Fatal("54분 + 횡단보도 190초는 55분을 넘는다")
	}
}

// 대중교통이 섞인 후보가 한도를 넘으면 나누지 않고 뺀다. 한도 안인 후보는 남는다.
func TestPlanDropsLongRentalWithTransit(t *testing.T) {
	start := time.Date(2026, 9, 14, 14, 0, 0, 0, kst)
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		if r.Modes.Transit != nil && len(r.Modes.Transit.Modes) > 0 {
			return nil, otp.ErrNoRoute
		}
		mixed := bikeTrip(start, 70, eastLine())
		mixed.Legs = append(mixed.Legs, otp.Leg{Mode: "SUBWAY", Route: "2호선", TransitLeg: true})
		short := bikeTrip(start, 30, "")
		return []otp.Itinerary{mixed, short}, nil
	}}
	its, err := (&Planner{OTP: f, Bikes: splitBikes()}).Plan(context.Background(),
		PlanRequest{Origin: rentFrom, Destination: rentTo, BikeLimitMin: 60})
	if err != nil {
		t.Fatal(err)
	}
	if got := rentedMinutes(its[0]); len(its) != 1 || got[0] != 30 {
		t.Fatalf("30분 대여 후보만 남아야: %+v", its)
	}
	for _, c := range f.calls {
		if isBikeOnly(c) {
			t.Fatalf("대중교통이 섞인 후보는 다시 탐색하지 않는다: %+v", c)
		}
	}
}

// 한도 안(55분 전)에 닿는 대여소가 없으면 나누지 못하고 뺀다.
func TestPlanDropsLongRentalWithoutStation(t *testing.T) {
	far := fakeBikes{snap: &gbfs.Snapshot{Stations: []gbfs.Station{{ID: "S3", Lat: 37.5005, Lon: 127.08}}}}
	_, err := (&Planner{OTP: splitOTP(70), Bikes: far}).Plan(context.Background(),
		PlanRequest{Origin: rentFrom, Destination: rentTo, BikeLimitMin: 60})
	if err == nil {
		t.Fatal("나눌 대여소가 없으면 후보가 없어 경로 없음이어야")
	}
}

// 구간 수단을 따릉이로 고정한 경로(구간별 탐색)도 구간 안에서 나눈다.
func TestSegmentedSplitsLongRental(t *testing.T) {
	its, err := (&Planner{OTP: splitOTP(70), Bikes: splitBikes()}).Plan(context.Background(),
		PlanRequest{Origin: rentFrom, Destination: rentTo, Modes: []SegmentMode{ModeBike}, BikeLimitMin: 60})
	if err != nil {
		t.Fatal(err)
	}
	if got := rentedMinutes(its[0]); len(its) != 1 || len(got) != 2 {
		t.Fatalf("구간 고정도 45분 + 25분으로 나뉘어야: %+v", its)
	}
}
