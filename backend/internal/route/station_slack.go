package route

import (
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 역 ID 앵커링에서 빠지는 출입구↔승강장 시간을 보정한다.
// 변경 시 gtfs/internal/build/pathways.go와 값을 맞추고 그래프를 재생성한다.
const (
	StationEntrySec = 120.0
	StationExitSec  = 60.0
)

// applyStationSlack 은 앵커링된 출발지의 여정 출발(Start)을 진입시간만큼 앞당기고(출입구에서 출발),
// 앵커링된 도착지의 도착(End)을 이탈시간만큼 늦춘다(출입구 도착). Duration 은 그만큼 늘어난다.
// leg 의 시각은 그대로라 첫 leg 앞·끝 leg 뒤에 그만큼 틈이 생긴다(실시간 보정은 그 틈을 접근시간으로 센다).
func applyStationSlack(its []otp.Itinerary, originAnchored, destAnchored bool) {
	for i := range its {
		it := &its[i]
		if originAnchored {
			if s, err := time.Parse(time.RFC3339, it.Start); err == nil {
				it.Start = s.Add(-time.Duration(StationEntrySec) * time.Second).Format(time.RFC3339)
				it.Duration += StationEntrySec
			}
		}
		if destAnchored {
			if e, err := time.Parse(time.RFC3339, it.End); err == nil {
				it.End = e.Add(time.Duration(StationExitSec) * time.Second).Format(time.RFC3339)
				it.Duration += StationExitSec
			}
		}
	}
}

// entryDepart 는 앵커링된 출발지의 OTP 요청 시각. 출입구에서 승강장까지 가는 동안 떠나는 차는 못 타므로
// 요청 시각(nil 이면 now)을 진입시간만큼 뒤로 민다. OTP 는 이 정도 차이는 "지금 출발" 로 보고 따릉이 잔여대수를 계속 반영한다.
func entryDepart(depart *time.Time, now time.Time) *time.Time {
	t := now
	if depart != nil {
		t = *depart
	}
	t = t.Add(time.Duration(StationEntrySec) * time.Second)
	return &t
}
