package realtime

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// MaxItineraries: 검색당 실시간 조회를 거는 상위 후보 수. 노선·역 캐시가 있어 실제 콜은 이보다 적다.
const MaxItineraries = 8

// Corrector 는 itinerary 의 첫 대중교통 탑승 대기를 실시간 도착으로 바꾼다. Bus/Subway 가 nil 이면 그 수단은 건너뛴다.
type Corrector struct {
	Bus    *BusClient
	Subway *SubwayClient
	Log    *slog.Logger
	Now    func() time.Time // 테스트용. nil 이면 time.Now
}

// Adjust 는 "지금 출발" 후보들의 첫 탑승을 보정한 사본을 돌려준다. 실패한 후보는 시간표 값 그대로.
func (c *Corrector) Adjust(ctx context.Context, its []otp.Itinerary) []otp.Itinerary {
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	out := append([]otp.Itinerary(nil), its...)
	for i := range out {
		if i >= MaxItineraries {
			break
		}
		if it, ok := c.adjustOne(ctx, now, out[i]); ok {
			out[i] = it
		}
	}
	return out
}

func (c *Corrector) adjustOne(ctx context.Context, now time.Time, it otp.Itinerary) (otp.Itinerary, bool) {
	k := -1
	for i, l := range it.Legs {
		if l.TransitLeg {
			k = i
			break
		}
	}
	if k < 0 {
		return it, false
	}
	leg := it.Legs[k]
	sched, err := time.Parse(time.RFC3339, leg.Start)
	if err != nil {
		return it, false
	}
	var access float64 // 탑승 정류장까지 걸리는 시간(초)
	for _, l := range it.Legs[:k] {
		access += l.Duration
	}
	earliest := now.Add(time.Duration(access) * time.Second) // 이보다 먼저 오는 차는 못 탄다
	var eta []int
	switch {
	case leg.Mode == "BUS" && c.Bus != nil:
		eta = c.busETA(ctx, leg)
	case (leg.Mode == "SUBWAY" || leg.Mode == "RAIL") && c.Subway != nil:
		eta = c.subwayETA(ctx, leg)
	default:
		return it, false
	}
	board := time.Time{}
	for _, s := range eta {
		t := now.Add(time.Duration(s) * time.Second)
		if !t.Before(earliest) {
			board = t
			break
		}
	}
	if board.IsZero() {
		return it, false
	}
	delta := board.Sub(sched)
	// 탑승 이후 leg 는 delta 만큼 옮기고(차내·환승 시간은 시간표 값 유지), 탑승 전 leg 와 출발도 같이 당기되
	// 출발이 now 보다 앞서지는 않게 한다(그만큼은 정류장에서 기다린다). 소요시간은 옮긴 Start·End 로 다시 잰다.
	start, err := time.Parse(time.RFC3339, it.Start)
	if err != nil {
		return it, false
	}
	pre := delta
	if start.Add(delta).Before(now) {
		pre = now.Sub(start)
	}
	for i := range it.Legs {
		d := delta
		if i < k {
			d = pre
		}
		it.Legs[i].Start = shift(it.Legs[i].Start, d)
		it.Legs[i].End = shift(it.Legs[i].End, d)
	}
	it.Start = shift(it.Start, pre)
	it.End = shift(it.End, delta)
	if s, e1 := time.Parse(time.RFC3339, it.Start); e1 == nil {
		if e, e2 := time.Parse(time.RFC3339, it.End); e2 == nil {
			it.Duration = e.Sub(s).Seconds()
		}
	}
	it.Realtime = true
	it.RealtimeDelta = delta.Seconds()
	return it, true
}

// busETA: RouteID "seoul:B_100100063" → busRouteId, FromStopID "seoul:BS_123000354" → stId.
func (c *Corrector) busETA(ctx context.Context, leg otp.Leg) []int {
	rid, ok1 := strings.CutPrefix(afterColon(leg.RouteID), "B_")
	sid, ok2 := strings.CutPrefix(afterColon(leg.FromStopID), "BS_")
	if !ok1 || !ok2 {
		return nil
	}
	a, found, err := c.Bus.Arrival(ctx, rid, sid)
	if err != nil {
		c.warn("bus arrival", err)
		return nil
	}
	if !found {
		return nil
	}
	return a.ExpsSec
}

// subwayETA: 역 기준명("서울(4호선)" → "서울")의 열차 중 같은 노선(subwayId)·같은 방면(다음 정차역)인 것.
func (c *Corrector) subwayETA(ctx context.Context, leg otp.Leg) []int {
	id := SubwayIDFor(leg.Route)
	if id == "" {
		return nil
	}
	station := baseName(leg.FromName)
	next := baseName(leg.NextStop)
	trains, err := c.Subway.Trains(ctx, station)
	if err != nil {
		c.warn("subway arrival", err)
		return nil
	}
	var eta []int
	for _, t := range trains {
		if t.SubwayID == id && t.Known && baseName(t.NextStop) == next {
			eta = append(eta, t.ETASec)
		}
	}
	return eta
}

func (c *Corrector) warn(msg string, err error) {
	if c.Log != nil {
		c.Log.Warn(msg, "err", err)
	}
}

func afterColon(s string) string {
	if _, after, ok := strings.Cut(s, ":"); ok {
		return after
	}
	return s
}

// baseName 은 "서울(4호선)" → "서울", "신촌(경의중앙선)" → "신촌".
func baseName(name string) string {
	if i := strings.Index(name, "("); i >= 0 {
		name = name[:i]
	}
	return strings.TrimSpace(name)
}

func shift(rfc string, d time.Duration) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return rfc
	}
	return t.Add(d).Format(time.RFC3339)
}
