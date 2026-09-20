package realtime

import (
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// TransferSlackSec: 도보 leg 없는 환승(같은 역 승강장 이동)에 OTP 가 두는 여유(초). 그 이동은 leg 으로 나오지
// 않으므로 이 값이 그 자리를 대신한다. OTP 2.10 기본 transferSlack PT2M — router-config 에서 바꾸면 같이 바꾼다.
const TransferSlackSec = 120

// laterShifts 는 첫 탑승 leg k 가 d 만큼 옮겨졌을 때 k 뒤 leg 마다의 이동량과 도착(End) 이동량을 돌려준다.
//   - d 가 음수면 다음 대중교통 탑승부터 0.
//   - 앞뒤 대중교통 leg 가 둘 다 지하철·철도면 탑승 지점 도착 시각(boardWait 참조)이 원래 탑승 시각 이하일 때 0,
//     넘으면 그 leg 의 NextDepartures 중 그 시각 이후 첫 출발까지의 차이. 환승마다 이어서 따진다.
//   - 다음 출발 정보가 없거나 버스가 낀 환승을 만나면 그때의 이동량을 뒤에 그대로 쓰고 더 따지지 않는다.
func laterShifts(legs []otp.Leg, k int, d time.Duration) ([]time.Duration, time.Duration) {
	out := make([]time.Duration, len(legs))
	cur, exact := d, timetabled(legs[k])
	for i := k + 1; i < len(legs); i++ {
		if legs[i].TransitLeg {
			switch {
			case cur < 0:
				cur = 0
			case cur > 0 && exact && timetabled(legs[i]):
				cur, exact = connect(legs, i, cur)
			default:
				exact = false
			}
		}
		out[i] = cur
	}
	return out, cur
}

// connect 는 직전 leg 가 cur 만큼 늦어졌을 때 leg legs[i] 의 원래 차를 타면 (0, true), 못 타면 NextDepartures 중
// 탈 수 있는 첫 차까지의 이동량과 true 를 돌려준다. 시각을 못 읽거나 탈 수 있는 다음 차가 없으면 (cur, false).
func connect(legs []otp.Leg, i int, cur time.Duration) (time.Duration, bool) {
	l := legs[i]
	e, err1 := time.Parse(time.RFC3339, legs[i-1].End)
	b, err2 := time.Parse(time.RFC3339, l.Start)
	if err1 != nil || err2 != nil {
		return cur, false
	}
	ready := e.Add(cur + boardWait(legs, i))
	if !ready.After(b) {
		return 0, true
	}
	var best time.Time
	for _, s := range l.NextDepartures {
		if t, err := time.Parse(time.RFC3339, s); err == nil && !t.Before(ready) && (best.IsZero() || t.Before(best)) {
			best = t
		}
	}
	if best.IsZero() {
		return cur, false
	}
	return best.Sub(b), true
}

// boardWait 는 직전 leg 끝에서 탑승까지 더 걸리는 시간. 앞이 도보·자전거면 그 구간들의 신호 횡단보도 기대 대기
// 합, 앞이 바로 대중교통이면 승강장 이동 TransferSlackSec. 두 항은 서로 대신하므로 함께 더하지 않는다.
// 앞 leg 끝과 시각이 끊기는 자리에서 합산을 멈춘다 — 재탐색이 이어 붙인 자리이고, 그 앞 대기는 뒤 구간의 시각에
// 이미 들어 있다. 2026-09-20 실측(대표 OD 20쌍 × 3시각): 시각이 끊긴 도보→도보 10쌍이 전부 재탐색 여정이었고,
// 자전거 대여·반납으로 이어지는 78쌍은 간격이 모두 0 이라 이 판정에 걸리지 않는다.
func boardWait(legs []otp.Leg, i int) time.Duration {
	if legs[i-1].TransitLeg {
		return TransferSlackSec * time.Second
	}
	var acc float64
	for j := i - 1; j >= 0 && !legs[j].TransitLeg; j-- {
		acc += legs[j].CrossingWait
		if j > 0 && !legs[j-1].TransitLeg && !continues(legs[j-1].End, legs[j].Start) {
			break
		}
	}
	return time.Duration(acc * float64(time.Second))
}

// continues 는 앞 leg 끝과 뒤 leg 시작이 같은 시각인지. 시각을 못 읽으면 이어진 것으로 본다.
func continues(prevEnd, start string) bool {
	e, err1 := time.Parse(time.RFC3339, prevEnd)
	s, err2 := time.Parse(time.RFC3339, start)
	return err1 != nil || err2 != nil || !s.After(e)
}

// timetabled 는 시간표로 운행하는 지하철·철도 leg 인지. 생성 GTFS 의 철도 노선은 전부 route_type 1(SUBWAY)이다.
func timetabled(l otp.Leg) bool { return l.Mode == "SUBWAY" || l.Mode == "RAIL" }
