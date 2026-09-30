package route

import (
	"fmt"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 도착 시각 지정(PlanRequest.Arrive) 규칙.
//   - 경유지·구간 수단 고정이 없는 요청만 받는다(구간별 탐색·경유지 체류는 앞 구간 도착에 뒤 구간을 이어 붙인다).
//   - OTP 에 latestArrival 로 보낸다. 도착지가 역 ID 로 앵커링되면 승강장에서 내리는 후보(지하철 전용 변형과 전체 수단
//     변형의 지하철·철도 종착 후보)만 이탈시간(StationExitSec)만큼 앞당긴 시각으로 찾는다(search.go single).
//   - 횡단보도 대기로 첫 탑승을 놓치면 재탐색 대신 넘친 만큼 일찍 출발한다(여정 Start 를 앞당긴다). 대중교통이 없는
//     여정은 대기 합만큼 일찍 출발하고 도착은 그대로다. 놓침은 addCrossingWaits 가 처음 찾은 것 하나만 보며, 그것이
//     대중교통을 탄 뒤의 도보(환승)면 그 여정을 뺀다.
//   - 따릉이 한도를 넘는 대여는 나누지 않고 뺀다(나누기는 출발 시각에서 앞으로 다시 탐색한다).
//   - 실시간 보정은 하지 않는다.
//   - 대기를 더한 도착이 지정 시각보다 늦은 여정은 뺀다. 남은 여정 중 같은 구간 열(legSig)이면 늦게 출발하는 여정을
//     남긴다.

// validateArrive 는 도착 시각 요청이 받을 수 있는 형태인지 본다.
func validateArrive(req PlanRequest, now time.Time) error {
	if req.Arrive == nil {
		return nil
	}
	if req.Depart != nil {
		return fmt.Errorf("%w: depart 와 arrive 는 하나만", ErrBadRequest)
	}
	if len(req.Via) > 0 {
		return fmt.Errorf("%w: 도착 시각 지정은 경유지 없이만", ErrBadRequest)
	}
	for _, mode := range req.Modes {
		if mode != ModeAny {
			return fmt.Errorf("%w: 도착 시각 지정은 구간 수단 고정 없이만", ErrBadRequest)
		}
	}
	if !req.Arrive.After(now) {
		return fmt.Errorf("%w: 도착 시각이 지났습니다", ErrBadRequest)
	}
	return nil
}

// arriveCrossings 는 addCrossingWaits 를 거친 도착 시각 요청 여정의 출발을 대기만큼 앞당긴다. k·acc 는
// addCrossingWaits 가 돌려준 탑승 직전 leg 와 대기 합, end0 은 대기를 더하기 전 End. 뺄 여정이면 false.
func arriveCrossings(it *otp.Itinerary, k int, acc float64, end0 string) bool {
	if !hasTransit(*it) {
		it.End = end0
		it.Start = shiftRFC3339(it.Start, -it.CrossingWait)
		return true
	}
	if k < 0 {
		return true
	}
	for _, l := range it.Legs[:k] {
		if l.TransitLeg {
			return false
		}
	}
	if slack, ok := slackSec(it.Legs[k].End, it.Legs[k+1].Start); ok {
		it.Start = shiftRFC3339(it.Start, -(acc - slack))
	}
	return true
}

// byArrival 은 도착(End)이 arrive 보다 늦지 않은 여정만 남긴다.
func byArrival(its []otp.Itinerary, arrive time.Time) []otp.Itinerary {
	var kept []otp.Itinerary
	for _, it := range its {
		if end, err := time.Parse(time.RFC3339, it.End); err == nil && !end.After(arrive) {
			kept = append(kept, it)
		}
	}
	return kept
}

func shiftRFC3339(rfc string, sec float64) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return rfc
	}
	return t.Add(time.Duration(sec * float64(time.Second))).Format(time.RFC3339)
}
