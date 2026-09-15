package realtime

import (
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// TransferSlackSec: OTP 가 환승마다 직전 leg 끝과 다음 탑승 사이에 두는 최소 여유(초). OTP 2.10 기본 transferSlack
// PT2M 이고 otp/router-config.json·deploy/otp/router-config.json 은 이 값을 설정하지 않는다. 2026-09-15 대표 OD 20쌍
// × 5시각 환승 544건의 최소 간격이 120초였다(이슈 #55). router-config 에서 바꾸면 이 값도 같이 바꾼다.
const TransferSlackSec = 120

// laterShifts 는 첫 탑승 leg k 가 d 만큼 옮겨졌을 때 k 뒤 leg 마다의 이동량과 도착(End) 이동량을 돌려준다.
//   - d 가 음수면 다음 대중교통 탑승부터 0(이슈 #52).
//   - 앞뒤 대중교통 leg 가 둘 다 지하철·철도면 직전 leg 끝 + 이동량 + TransferSlackSec 가 원래 탑승 시각 이하일 때 0,
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
				cur, exact = connect(legs[i-1].End, legs[i], cur)
			default:
				exact = false
			}
		}
		out[i] = cur
	}
	return out, cur
}

// connect 는 직전 leg 끝(prevEnd)이 cur 만큼 늦어졌을 때 leg l 의 원래 차를 타면 (0, true), 못 타면 NextDepartures 중
// 탈 수 있는 첫 차까지의 이동량과 true 를 돌려준다. 시각을 못 읽거나 탈 수 있는 다음 차가 없으면 (cur, false).
func connect(prevEnd string, l otp.Leg, cur time.Duration) (time.Duration, bool) {
	e, err1 := time.Parse(time.RFC3339, prevEnd)
	b, err2 := time.Parse(time.RFC3339, l.Start)
	if err1 != nil || err2 != nil {
		return cur, false
	}
	ready := e.Add(cur + TransferSlackSec*time.Second)
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

// timetabled 는 시간표로 운행하는 지하철·철도 leg 인지. 생성 GTFS 의 철도 노선은 전부 route_type 1(SUBWAY)이다.
func timetabled(l otp.Leg) bool { return l.Mode == "SUBWAY" || l.Mode == "RAIL" }
