package otp

import (
	"math"

	"github.com/SIDED00R/seoul-route/backend/internal/crossing"
)

// trimStationWalk 는 역 안에서 끝나거나 시작하는 도보 구간을 출입구에서 자른다. 출입구 단계(ENTER_STATION·EXIT_STATION)
// 중 마지막이 ENTER_STATION 이면 역으로 들어가는 구간이라 그 출입구에서 끝나고(도착 이름 = 출입구 이름), 첫째가
// EXIT_STATION 이면 역에서 나오는 구간이라 그 출입구에서 시작한다. 역을 지나가기만 하는 도보(들어갔다 나옴)는 두
// 조건에 걸리지 않아 그대로다. 출입구와 승강장 사이 역 안 통로는 경로선·steps 에서 빼고, 거리(Distance)·시각(Start·End)·
// 소요는 OTP 값 그대로 둔다. 앱 안내는 구간 끝점 반경으로 다음 구간으로 넘기므로 출입구에 닿으면 대중교통 구간으로 넘어간다.
func trimStationWalk(leg *Leg) {
	if leg.Mode != "WALK" || len(leg.Steps) == 0 {
		return
	}
	var gates []int // 출입구 단계 번호
	for i, s := range leg.Steps {
		if s.Dir == "ENTER_STATION" || s.Dir == "EXIT_STATION" {
			gates = append(gates, i)
		}
	}
	if len(gates) == 0 {
		return
	}
	enter, exit := -1, -1
	if last := gates[len(gates)-1]; leg.Steps[last].Dir == "ENTER_STATION" {
		enter = last
	}
	if first := gates[0]; leg.Steps[first].Dir == "EXIT_STATION" {
		exit = first
	}
	if enter < 0 && exit < 0 {
		return
	}
	from, to := 0, len(leg.Steps)-1
	if exit >= 0 {
		from = exit
	}
	if enter >= 0 {
		to = enter
	}
	pts := crossing.DecodePolyline(leg.Polyline)
	lo, hi := 0, len(pts)-1
	if exit >= 0 {
		lo = nearestVertex(pts, 0, leg.Steps[exit])
	}
	if enter >= 0 {
		hi = nearestVertex(pts, lo, leg.Steps[enter])
	}
	if len(pts) >= 2 && hi > lo {
		leg.Polyline = encodePolyline(pts[lo : hi+1])
	}
	leg.Steps = append([]Step(nil), leg.Steps[from:to+1]...)
	if exit >= 0 {
		s := leg.Steps[0]
		leg.FromLat, leg.FromLon = s.Lat, s.Lon
		if s.Entrance != "" {
			leg.FromName = s.Entrance
		}
	}
	if enter >= 0 {
		s := leg.Steps[len(leg.Steps)-1]
		leg.ToLat, leg.ToLon = s.Lat, s.Lon
		if s.Entrance != "" {
			leg.ToName = s.Entrance
		}
	}
}

// nearestVertex 는 pts[start:] 중 단계 지점에 가장 가까운 점의 번호.
func nearestVertex(pts []crossing.Point, start int, s Step) int {
	best, bestD := start, math.Inf(1)
	for i := start; i < len(pts); i++ {
		dLat := (pts[i].Lat - s.Lat) * 111000
		dLon := (pts[i].Lon - s.Lon) * 88000
		if d := dLat*dLat + dLon*dLon; d < bestD {
			best, bestD = i, d
		}
	}
	return best
}
