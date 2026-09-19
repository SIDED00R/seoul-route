package route

import "github.com/SIDED00R/seoul-route/backend/internal/otp"

// annotateFastExits 는 지하철 leg 에 하차역의 설비 앞 칸-문을 붙인다. 승강장 방향은 하차 직전 정차역(중간 정차가
// 없으면 탑승역)으로 정한다.
func (p *Planner) annotateFastExits(its []otp.Itinerary) {
	if p.FastExits == nil {
		return
	}
	for i := range its {
		for j := range its[i].Legs {
			l := &its[i].Legs[j]
			if l.Mode != "SUBWAY" {
				continue
			}
			prev := l.FromName
			if n := len(l.Stops); n > 0 {
				prev = l.Stops[n-1].Name
			}
			l.FastExit = p.FastExits.Lookup(l.Route, l.ToName, prev)
		}
	}
}
