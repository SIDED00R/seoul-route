package route

import (
	"strings"

	"github.com/SIDED00R/seoul-route/backend/internal/gbfs"
	"github.com/SIDED00R/seoul-route/backend/internal/geo"
	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// BikeStations 는 따릉이 대여소 실시간 스냅샷을 주는 쪽(gbfs.Poller). stale 이면 붙이지 않는다.
type BikeStations interface {
	Current() (*gbfs.Snapshot, bool)
}

// BikeStationMatchM: leg 출발점에서 이만큼 안에 있는 대여소만 같은 곳으로 본다. OTP 는 우리가 만든 GBFS 피드의
// 좌표를 그대로 쓰므로 정상적으로는 0m 다. 이 값은 좌표 반올림·피드 갱신으로 어긋나는 경우만 흡수한다.
const BikeStationMatchM = 30.0

// annotateBikeStations 는 따릉이 대여 구간에 빌릴 대여소의 남은 자전거 대수를 붙인다. 대여소를 못 찾거나
// 스냅샷이 오래됐으면 붙이지 않는다(앱은 그때 대수를 보여 주지 않는다).
func (p *Planner) annotateBikeStations(its []otp.Itinerary) {
	if p.Bikes == nil {
		return
	}
	snap, stale := p.Bikes.Current()
	if snap == nil || stale {
		return
	}
	for i := range its {
		for j := range its[i].Legs {
			l := &its[i].Legs[j]
			if !l.RentedBike {
				continue
			}
			if s := findStation(snap.Stations, l.FromName, l.FromLat, l.FromLon); s != nil {
				l.BikesAvailable = s.Bikes
				l.HasBikeCount = true
			}
		}
	}
}

// findStation 은 이름이 같은 대여소를, 없으면 BikeStationMatchM 안의 가장 가까운 대여소를 돌려준다.
func findStation(stations []gbfs.Station, name string, lat, lon float64) *gbfs.Station {
	want := strings.TrimSpace(name)
	best := -1
	bestD := BikeStationMatchM
	for i := range stations {
		s := &stations[i]
		if strings.TrimSpace(s.Name) == want {
			return s
		}
		if d := geo.DistM(lat, lon, s.Lat, s.Lon); d <= bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return nil
	}
	return &stations[best]
}
