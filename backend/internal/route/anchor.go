package route

import (
	"math"
	"strings"

	"github.com/SIDED00R/seoul-route/backend/internal/geo"
)

// AnchorRadiusM: 장소명이 "…역" 일 때 같은 이름의 부모역을 찾는 반경. 카카오 "서울역"(역사 건물)과 GTFS "서울"
// 부모역(자식 평균)은 약 100m, 출입구 좌표라도 수백 m 라 1km 면 충분하고, 다른 동네의 동명 역은 걸러진다.
const AnchorRadiusM = 1000

// anchor 는 장소명이 역이면 그 역의 gtfsId 를, 아니면 "" 를 돌려준다.
// 역사 건물 좌표는 도로망에서 선로 반대편·승강장·지하상가에 붙어 도보가 1km 넘게 늘어난다(이슈 #12 실측).
// 좌표 대신 역 ID 를 주면 OTP 가 역 안에서 여정에 맞는 stop 을 고른다.
func (p *Planner) anchor(pt Point) string {
	base := stationBase(pt.Name)
	if base == "" {
		return ""
	}
	p.mu.RLock()
	stations := p.stations
	p.mu.RUnlock()
	bestID, bestD := "", math.Inf(1)
	for _, s := range stations {
		if s.Name != base {
			continue
		}
		if d := geo.DistM(pt.Lat, pt.Lon, s.Lat, s.Lon); d <= AnchorRadiusM && d < bestD {
			bestID, bestD = s.ID, d
		}
	}
	return bestID
}

// stationBase 는 카카오 장소명에서 역 기준명을 뽑는다. "서울역" → "서울", "강남역 2호선" → "강남",
// "서울역 공항철도" → "서울". 첫 낱말이 "역" 으로 끝나지 않으면(카페·편의점 등) "" 를 돌려준다.
func stationBase(name string) string {
	first := strings.Fields(strings.TrimSpace(name))
	if len(first) == 0 {
		return ""
	}
	w := first[0]
	if !strings.HasSuffix(w, "역") || len(w) <= len("역") {
		return ""
	}
	return strings.TrimSuffix(w, "역")
}
