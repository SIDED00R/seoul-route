package realtime

import (
	"context"
	"strings"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 버스 "운행종료" 로 후보를 빼는 탑승 예정 시각 범위(막차대, KST): lastBusFromHour 시 이후이거나 lastBusToHour 시 전.
// 심야버스(노선 번호가 N 으로 시작)는 빼지 않는다 — 첫 출발이 막차대 안이다(N26 00:07·01:53).
// 출처: 생성 GTFS 버스 중 심야버스 18노선·36방향을 뺀 2,004 방향(2026-09-29) — 정류장별 실제 막차 시각의 마지막 출발 p50 24:09·
// p99 26:34, 03:00 넘어 가는 방향 5개, 12:00 전에 끝나는 방향 23개, 첫 출발이 03:00 전인 방향 0개. 버스 API 를 다시
// 받으면(gtfsgen fetch) 정류장별 lastTm 으로 다시 잰다.
const (
	lastBusFromHour = 12
	lastBusToHour   = 3
)

// endedDrops 는 첫 탑승 버스 leg 가 sched 에 탈 예정일 때 그 정류장의 "운행종료" 로 후보를 빼는지.
func endedDrops(leg otp.Leg, sched time.Time) bool {
	h := sched.Hour()
	return (h >= lastBusFromHour || h < lastBusToHour) && !strings.HasPrefix(leg.Route, "N")
}

// firstBusEnded 는 첫 대중교통 탑승이 버스이고 endedDrops 조건에서 그 정류장이 실시간 "운행종료" 인지. 보정
// 상한(MaxItineraries) 밖 후보에 쓴다.
func (c *Corrector) firstBusEnded(ctx context.Context, it otp.Itinerary) bool {
	for _, l := range it.Legs {
		if !l.TransitLeg {
			continue
		}
		if l.Mode != "BUS" || c.Bus == nil {
			return false
		}
		sched, err := time.Parse(time.RFC3339, l.Start)
		if err != nil || !endedDrops(l, sched) {
			return false
		}
		_, ended := c.busETA(ctx, l)
		return ended
	}
	return false
}
