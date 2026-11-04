package route

import (
	"math"

	"github.com/SIDED00R/seoul-route/backend/internal/geo"
)

// 열차 안 출발지(Point.OnBoard) 규칙.
//   - 이름과 상관없이 좌표에서 OnBoardRadiusM 안의 가장 가까운 부모역으로 앵커링한다. 없으면 경로 없음이다 — 좌표
//     그대로 찾으면 열차 안에서 지상을 걸어 나가는 경로가 나온다.
//   - 이미 승강장(열차)에 있으므로 출입구→승강장 진입시간(StationEntrySec)을 요청 시각과 여정 출발에 붙이지 않는다.

// OnBoardRadiusM 은 열차 안 위치에서 역을 찾는 반경이다. 초기값(2026-11-04): 운영 안내 궤적 2건에서 지하 위치는
// 대부분 역에서 와이파이로 잡혔고 정확도 10~30m 였다. 역사 크기와 위치 오차를 덮되 역 간격(대개 1km 안팎)의 절반을
// 넘지 않게 300m 로 둔다.
const OnBoardRadiusM = 300

// nearestStation 은 pt 에서 radiusM 안의 가장 가까운 부모역 gtfsId 를, 없으면 "" 를 돌려준다.
func (p *Planner) nearestStation(pt Point, radiusM float64) string {
	p.mu.RLock()
	stations := p.stations
	p.mu.RUnlock()
	bestID, bestD := "", math.Inf(1)
	for _, s := range stations {
		if d := geo.DistM(pt.Lat, pt.Lon, s.Lat, s.Lon); d <= radiusM && d < bestD {
			bestID, bestD = s.ID, d
		}
	}
	return bestID
}

// entersStation 은 앵커링된 출발지에서 출입구→승강장 진입시간을 붙여야 하는지. 열차 안 출발지는 붙이지 않는다.
func entersStation(origin Point) bool { return !origin.OnBoard }
