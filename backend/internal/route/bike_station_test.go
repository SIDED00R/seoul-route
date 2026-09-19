package route

import (
	"context"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/gbfs"
	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

type fakeBikes struct {
	snap  *gbfs.Snapshot
	stale bool
}

func (f fakeBikes) Current() (*gbfs.Snapshot, bool) { return f.snap, f.stale }

func bikeSnap() *gbfs.Snapshot {
	return &gbfs.Snapshot{FetchedAt: time.Now(), Stations: []gbfs.Station{
		{ID: "ST-4", Name: "807. 서울역 12번 출구 앞", Lat: 37.5547, Lon: 126.9707, Capacity: 15, Bikes: 3},
		{ID: "ST-5", Name: "4615. 크라운호텔앞 버스정류소", Lat: 37.5326, Lon: 126.9906, Capacity: 14, Bikes: 0},
		{ID: "ST-6", Name: "이름이 다른 대여소", Lat: 37.5600, Lon: 126.9800, Capacity: 13, Bikes: 7},
	}}
}

func bikeItin(legs ...otp.Leg) []otp.Itinerary {
	return []otp.Itinerary{itin("2026-09-19T09:00:00+09:00", "2026-09-19T09:30:00+09:00", legs...)}
}

// 따릉이 대여 구간에는 이름이 같은 대여소의 남은 대수가 붙는다. 0대도 "모름" 과 구분해 싣는다.
func TestPlanAnnotatesBikeStations(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return bikeItin(
			otp.Leg{Mode: "WALK", ToName: "807. 서울역 12번 출구 앞"},
			otp.Leg{Mode: "BICYCLE", RentedBike: true, FromName: "807. 서울역 12번 출구 앞",
				FromLat: 37.5547, FromLon: 126.9707, ToName: "4615. 크라운호텔앞 버스정류소"},
			otp.Leg{Mode: "BICYCLE", RentedBike: true, FromName: "4615. 크라운호텔앞 버스정류소",
				FromLat: 37.5326, FromLon: 126.9906, ToName: "어딘가"},
			otp.Leg{Mode: "BICYCLE", FromName: "807. 서울역 12번 출구 앞", FromLat: 37.5547, FromLon: 126.9707}), nil
	}}
	p := &Planner{OTP: f, Bikes: fakeBikes{snap: bikeSnap()}}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil {
		t.Fatal(err)
	}
	legs := its[0].Legs
	if !legs[1].HasBikeCount || legs[1].BikesAvailable != 3 {
		t.Errorf("대여 구간: %+v", legs[1])
	}
	if !legs[2].HasBikeCount || legs[2].BikesAvailable != 0 { // 0대도 싣는다
		t.Errorf("빈 대여소: has=%v n=%d", legs[2].HasBikeCount, legs[2].BikesAvailable)
	}
	if legs[0].HasBikeCount || legs[3].HasBikeCount { // 도보·내 자전거에는 안 붙는다
		t.Errorf("따릉이가 아닌 leg: %+v %+v", legs[0], legs[3])
	}
}

// 이름이 안 맞으면 좌표로 찾되 BikeStationMatchM 밖이면 붙이지 않는다.
func TestAnnotateBikeStationsByCoords(t *testing.T) {
	snap := bikeSnap()
	cases := []struct {
		name     string
		lat, lon float64
		want     int // -1 = 붙이지 않음
	}{
		{"대여소 바로 위", 37.5600, 126.9800, 7},
		{"20m 옆", 37.56018, 126.9800, 7},
		{"200m 밖", 37.5618, 126.9800, -1},
	}
	for _, c := range cases {
		f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
			return bikeItin(otp.Leg{Mode: "BICYCLE", RentedBike: true, FromName: "OTP 가 준 다른 이름",
				FromLat: c.lat, FromLon: c.lon}), nil
		}}
		p := &Planner{OTP: f, Bikes: fakeBikes{snap: snap}}
		its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
		if err != nil {
			t.Fatal(err)
		}
		l := its[0].Legs[0]
		if c.want < 0 && l.HasBikeCount {
			t.Errorf("%s: 붙으면 안 되는데 %d대", c.name, l.BikesAvailable)
		}
		if c.want >= 0 && (!l.HasBikeCount || l.BikesAvailable != c.want) {
			t.Errorf("%s: has=%v n=%d", c.name, l.HasBikeCount, l.BikesAvailable)
		}
	}
}

// 실시간 값이 오래됐거나 없으면 대수를 붙이지 않는다(틀린 수를 보여 주지 않는다).
func TestAnnotateBikeStationsSkipsStale(t *testing.T) {
	for _, c := range []struct {
		name  string
		bikes BikeStations
	}{
		{"stale", fakeBikes{snap: bikeSnap(), stale: true}},
		{"스냅샷 없음", fakeBikes{snap: nil, stale: true}},
		{"폴러 없음", nil},
	} {
		f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
			return bikeItin(otp.Leg{Mode: "BICYCLE", RentedBike: true, FromName: "807. 서울역 12번 출구 앞",
				FromLat: 37.5547, FromLon: 126.9707}), nil
		}}
		p := &Planner{OTP: f, Bikes: c.bikes}
		its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
		if err != nil {
			t.Fatal(err)
		}
		if its[0].Legs[0].HasBikeCount {
			t.Errorf("%s: 대수가 붙었다", c.name)
		}
	}
}
