package route

import (
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
	"github.com/SIDED00R/seoul-route/backend/internal/realtime"
)

// 역 ID 앵커링에서 빠지는 출입구↔승강장 시간을 보정한다.
// 변경 시 gtfs/internal/build/pathways.go와 값을 맞추고 그래프를 재생성한다.
const (
	StationEntrySec = 120.0
	StationExitSec  = 60.0
)

// ViaTransferSec 는 역 ID 로 앵커링된 경유지(체류 0)에서 구간별 탐색이 다음 구간 출발에 두는 환승 여유(초).
// realtime.TransferSlackSec(OTP transferSlack)과 같은 값.
const ViaTransferSec = realtime.TransferSlackSec

// boardsStationFirst 는 첫 leg 가 그 역 승강장에서 바로 타는 지하철·철도인지. 앵커링된 출발지에서 OTP 는 이런 후보만
// 승강장에서 시작하고, 나머지(도보·자전거·버스)는 출입구에서 걷기 시작한다.
func boardsStationFirst(it otp.Itinerary) bool {
	return len(it.Legs) > 0 && it.Legs[0].TransitLeg && railMode(it.Legs[0].Mode)
}

// alightsStationLast 는 마지막 leg 가 그 역 승강장에서 내리는 지하철·철도인지.
func alightsStationLast(it otp.Itinerary) bool {
	return len(it.Legs) > 0 && it.Legs[len(it.Legs)-1].TransitLeg && railMode(it.Legs[len(it.Legs)-1].Mode)
}

func railMode(mode string) bool { return mode == "SUBWAY" || mode == "RAIL" }

// applyStationSlack 은 앵커링된 출발지에서 승강장에서 바로 타는 후보의 여정 출발(Start)을 진입시간만큼 앞당기고
// (출입구에서 출발), 앵커링된 도착지에서 승강장에서 내리는 후보의 도착(End)을 이탈시간만큼 늦춘다(출입구 도착).
// Duration 은 그만큼 늘어나고, leg 의 시각은 그대로라 첫 leg 앞·끝 leg 뒤에 틈이 생긴다(실시간 보정은 그 틈을
// 접근시간으로 센다). 승강장에 가지 않는 후보에는 붙이지 않는다. 출발 시각 요청(base = 요청 시각 또는 now)에서 대중교통이
// 없는 후보는 entryDepart 로 밀린 만큼(base 부터 첫 leg 시작까지, 최대 진입시간) 전 leg 를 앞당겨 요청 시각에 출발하게
// 한다. 도착 시각 요청(base nil)은 요청 시각을 밀지 않았으므로 되돌리지 않는다.
func applyStationSlack(its []otp.Itinerary, originAnchored, destAnchored bool, base *time.Time) {
	for i := range its {
		it := &its[i]
		if originAnchored {
			switch {
			case boardsStationFirst(*it):
				if s, err := time.Parse(time.RFC3339, it.Start); err == nil {
					it.Start = s.Add(-time.Duration(StationEntrySec) * time.Second).Format(time.RFC3339)
					it.Duration += StationEntrySec
				}
			case base == nil:
				// 도착 시각 요청은 entryDepart 로 밀지 않았으므로 되돌릴 것이 없다.
			case !hasTransit(*it):
				if s, err := time.Parse(time.RFC3339, it.Legs[0].Start); err == nil {
					back := s.Sub(*base)
					if back > time.Duration(StationEntrySec)*time.Second {
						back = time.Duration(StationEntrySec) * time.Second
					}
					if back > 0 {
						shiftItinerary(it, -back)
					}
				}
			}
		}
		if destAnchored && alightsStationLast(*it) {
			if e, err := time.Parse(time.RFC3339, it.End); err == nil {
				it.End = e.Add(time.Duration(StationExitSec) * time.Second).Format(time.RFC3339)
				it.Duration += StationExitSec
			}
		}
	}
}

// shiftItinerary 는 여정과 모든 leg 의 시각을 d 만큼 옮긴다.
func shiftItinerary(it *otp.Itinerary, d time.Duration) {
	it.Start = shiftRFC3339(it.Start, d.Seconds())
	it.End = shiftRFC3339(it.End, d.Seconds())
	for k := range it.Legs {
		it.Legs[k].Start = shiftRFC3339(it.Legs[k].Start, d.Seconds())
		it.Legs[k].End = shiftRFC3339(it.Legs[k].End, d.Seconds())
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
