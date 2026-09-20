package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 도보(300) → 402 → 도보 → 버스 → 도보 → 버스 → 도보(환승 2회). b = 시간표 첫 탑승(초, now 기준).
func transferItinerary(b int) otp.Itinerary {
	legs := []otp.Leg{
		{Mode: "WALK", Duration: 300, Start: at(b - 300), End: at(b)},
		{Mode: "BUS", Route: "402", RouteID: "seoul:B_100100063", FromStopID: "seoul:BS_1", TransitLeg: true,
			Start: at(b), End: at(b + 1200)},
		{Mode: "WALK", Duration: 120, Start: at(b + 1200), End: at(b + 1320)},
		{Mode: "BUS", Route: "B2", RouteID: "seoul:B_2", FromStopID: "seoul:BS_20", TransitLeg: true,
			Start: at(b + 1440), End: at(b + 2640)},
		{Mode: "WALK", Duration: 60, Start: at(b + 2640), End: at(b + 2700)},
		{Mode: "BUS", Route: "B3", RouteID: "seoul:B_3", FromStopID: "seoul:BS_30", TransitLeg: true,
			Start: at(b + 2820), End: at(b + 4020)},
		{Mode: "WALK", Duration: 300, Start: at(b + 4020), End: at(b + 4320)},
	}
	return otp.Itinerary{Start: at(b - 300), End: at(b + 4320), Duration: 4620, Transfers: 2, Legs: legs}
}

// 첫 차가 시간표보다 이르면(접근 300초 → 120초 차는 놓치고 660초 차, 시간표 1710초 → delta −1050)
// 첫 탑승과 그 뒤 환승 도보까지만 당기고, 둘째 탑승부터 도착은 시간표 그대로 둔다. RealtimeDelta 는 도착 이동량 0.
func TestEarlierFirstBusKeepsLaterTransfers(t *testing.T) {
	var calls int32
	srv := fakeBus(t, &calls)
	defer srv.Close()
	c := &Corrector{Bus: &BusClient{Key: "k", HTTP: srv.Client(), Base: srv.URL}, Now: func() time.Time { return now }}
	in := transferItinerary(1710)
	got := c.Adjust(context.Background(), []otp.Itinerary{in})[0]
	if !got.Realtime || got.RealtimeDelta != 0 {
		t.Errorf("도착 변화 0 이어야: realtime=%v delta=%v", got.Realtime, got.RealtimeDelta)
	}
	if got.End != in.End || got.Legs[3].Start != in.Legs[3].Start || got.Legs[5].Start != in.Legs[5].Start ||
		got.Legs[6].End != in.Legs[6].End {
		t.Errorf("뒤 탑승·도착은 시간표 그대로여야: End %s→%s, 둘째 탑승 %s→%s", in.End, got.End, in.Legs[3].Start,
			got.Legs[3].Start)
	}
	if got.Legs[1].Start != at(660) || got.Legs[2].End != at(1980) || got.Start != at(360) || got.Duration != 5670 {
		t.Errorf("첫 탑승·환승 도보·출발만 −1050: start=%s board=%s walkEnd=%s dur=%v",
			got.Start, got.Legs[1].Start, got.Legs[2].End, got.Duration)
	}
}

// 첫 차가 시간표보다 늦으면(delta +60) 뒤 구간과 도착도 같이 민다.
func TestLaterFirstBusStillShiftsTail(t *testing.T) {
	var calls int32
	srv := fakeBus(t, &calls)
	defer srv.Close()
	c := &Corrector{Bus: &BusClient{Key: "k", HTTP: srv.Client(), Base: srv.URL}, Now: func() time.Time { return now }}
	in := transferItinerary(600)
	got := c.Adjust(context.Background(), []otp.Itinerary{in})[0]
	if got.RealtimeDelta != 60 || got.End != at(600+4320+60) || got.Legs[3].Start != at(600+1440+60) ||
		got.Start != at(360) || got.Duration != 4620 {
		t.Errorf("+60 은 뒤까지 전부 이동: delta=%v end=%s 둘째=%s start=%s dur=%v", got.RealtimeDelta, got.End,
			got.Legs[3].Start, got.Start, got.Duration)
	}
}
