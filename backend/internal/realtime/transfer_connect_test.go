package realtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 서울역 4호선 회현방면 열차 하나가 eta 초 뒤에 온다.
func line4Arrival(t *testing.T, eta int) *Corrector {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"errorMessage":{"status":200,"code":"INFO-000","message":"ok"},"realtimeArrivalList":[
		 {"subwayId":"1004","trainLineNm":"불암산행 - 회현방면","barvlDt":"%d","arvlCd":"99"}]}`, eta)
	}))
	t.Cleanup(srv.Close)
	return &Corrector{Subway: &SubwayClient{Key: "s", HTTP: srv.Client(), Base: srv.URL},
		Now: func() time.Time { return now }}
}

// 도보(540~600) → 4호선(600~1200) → 환승 도보(1200~1320) → 3호선(1500~2100, 여유 180초) → 도보(2100~2160).
func subwayTransferItinerary() otp.Itinerary {
	legs := []otp.Leg{
		{Mode: "WALK", Duration: 60, Start: at(540), End: at(600)},
		{Mode: "SUBWAY", Route: "서울4호선", RouteID: "seoul:RR_4", FromName: "서울(4호선)", NextStop: "회현(4호선)",
			TransitLeg: true, Start: at(600), End: at(1200), NextDepartures: []string{at(900)}},
		{Mode: "WALK", Duration: 120, Start: at(1200), End: at(1320)},
		{Mode: "SUBWAY", Route: "서울3호선", RouteID: "seoul:RR_3", FromName: "충무로(3호선)", TransitLeg: true,
			Start: at(1500), End: at(2100), NextDepartures: []string{at(1800), at(2100)}},
		{Mode: "WALK", Duration: 60, Start: at(2100), End: at(2160)},
	}
	return otp.Itinerary{Start: at(540), End: at(2160), Duration: 1620, Transfers: 1, Legs: legs}
}

// 재탐색이 이어 붙인 자리(앞 도보 끝 1320 과 뒤 도보 시작 1380 사이가 끊긴다) 앞의 대기는 이미 뒤 구간 시각에
// 들어 있으므로 다시 더하지 않는다. 지연 60초면 1430+60 = 1490 ≤ 1500 이라 그대로인데, 대기 114 를 다시 더하면
// 1604 > 1500 이라 탈 수 있는 열차를 놓쳤다고 본다.
func TestBoardWaitStopsAtReplanSplice(t *testing.T) {
	in := subwayTransferItinerary()
	in.Legs[2].CrossingWait = 114
	in.Legs = append(in.Legs[:3], append([]otp.Leg{
		{Mode: "WALK", Duration: 50, Start: at(1380), End: at(1430)},
	}, in.Legs[3:]...)...)
	in.Legs[4].NextDepartures = []string{at(1800)}
	in.Replanned = true
	got := line4Arrival(t, 660).Adjust(context.Background(), []otp.Itinerary{in})[0] // 지연 60초
	if got.RealtimeDelta != 0 || got.Legs[4].Start != at(1500) {
		t.Fatalf("이음매 앞 대기를 다시 더했다: delta=%v 둘째=%s", got.RealtimeDelta, got.Legs[4].Start)
	}
}

// 첫 열차 +180초 = 환승 여유(180초) 경계 → 둘째 열차·도착은 시간표 그대로, 출발만 +180.
func TestSubwayDelayWithinSlackKeepsConnection(t *testing.T) {
	in := subwayTransferItinerary()
	got := line4Arrival(t, 780).Adjust(context.Background(), []otp.Itinerary{in})[0]
	if !got.Realtime || got.RealtimeDelta != 0 || got.End != in.End || got.Legs[3].Start != in.Legs[3].Start ||
		got.Legs[1].Start != at(780) || got.Legs[2].End != at(1500) || got.Start != at(720) || got.Duration != 1440 {
		t.Fatalf("여유 안 지연은 도착 불변: delta=%v end=%s 둘째=%s start=%s dur=%v", got.RealtimeDelta, got.End,
			got.Legs[3].Start, got.Start, got.Duration)
	}
}

// 첫 열차 +190초 > 여유 180초 → 도보 끝 1510 이후 첫 3호선 1800 → 도착 +300(지연 190초보다 더 늦다).
func TestSubwayDelayBeyondSlackTakesNextTrain(t *testing.T) {
	got := line4Arrival(t, 790).Adjust(context.Background(), []otp.Itinerary{subwayTransferItinerary()})[0]
	if got.RealtimeDelta != 300 || got.Legs[3].Start != at(1800) || got.End != at(2460) || got.Legs[2].End != at(1510) ||
		got.Start != at(730) || got.Duration != 1730 {
		t.Fatalf("다음 열차 기준: delta=%v 둘째=%s end=%s dur=%v", got.RealtimeDelta, got.Legs[3].Start, got.End, got.Duration)
	}
}

// 환승 도보의 신호 횡단보도 대기도 여유를 먹는다. 대기 없으면 +110 은 여유 안, 대기 76 이면 1506 > 1500.
func TestSubwayTransferWalkCrossingWaitTakesNextTrain(t *testing.T) {
	in := subwayTransferItinerary()
	kept := line4Arrival(t, 710).Adjust(context.Background(), []otp.Itinerary{in})[0]
	if kept.RealtimeDelta != 0 || kept.Legs[3].Start != at(1500) {
		t.Fatalf("대기 없으면 그대로: delta=%v 둘째=%s", kept.RealtimeDelta, kept.Legs[3].Start)
	}
	in = subwayTransferItinerary()
	in.Legs[2].CrossingWait = 76
	got := line4Arrival(t, 710).Adjust(context.Background(), []otp.Itinerary{in})[0]
	if got.RealtimeDelta != 300 || got.Legs[3].Start != at(1800) {
		t.Fatalf("대기가 여유를 먹는다: delta=%v 둘째=%s", got.RealtimeDelta, got.Legs[3].Start)
	}
}

// 도보 leg 없는 환승은 TransferSlackSec 을 더한다. 여유 200초: +60 → 1380 ≤ 1400 그대로, +100 → 1420 > 1400.
func TestSubwayNoWalkTransferUsesTransferSlack(t *testing.T) {
	plain := func() otp.Itinerary {
		legs := []otp.Leg{
			{Mode: "WALK", Duration: 60, Start: at(540), End: at(600)},
			{Mode: "SUBWAY", Route: "서울4호선", RouteID: "seoul:RR_4", FromName: "서울(4호선)", NextStop: "회현(4호선)",
				TransitLeg: true, Start: at(600), End: at(1200), NextDepartures: []string{at(900)}},
			{Mode: "SUBWAY", Route: "서울3호선", RouteID: "seoul:RR_3", FromName: "충무로(3호선)", TransitLeg: true,
				Start: at(1400), End: at(2000), NextDepartures: []string{at(1700)}},
			{Mode: "WALK", Duration: 60, Start: at(2000), End: at(2060)},
		}
		return otp.Itinerary{Start: at(540), End: at(2060), Duration: 1520, Transfers: 1, Legs: legs}
	}
	kept := line4Arrival(t, 660).Adjust(context.Background(), []otp.Itinerary{plain()})[0]
	if kept.RealtimeDelta != 0 || kept.Legs[2].Start != at(1400) {
		t.Fatalf("여유 안: delta=%v 둘째=%s", kept.RealtimeDelta, kept.Legs[2].Start)
	}
	got := line4Arrival(t, 700).Adjust(context.Background(), []otp.Itinerary{plain()})[0]
	if got.RealtimeDelta != 300 || got.Legs[2].Start != at(1700) {
		t.Fatalf("승강장 이동 120초를 넘김: delta=%v 둘째=%s", got.RealtimeDelta, got.Legs[2].Start)
	}
}

// 둘째 leg 의 다음 출발 정보가 없으면 지연만큼만 민다.
func TestSubwayNoNextDepartureFallsBackToDelta(t *testing.T) {
	in := subwayTransferItinerary()
	in.Legs[3].NextDepartures = nil
	got := line4Arrival(t, 790).Adjust(context.Background(), []otp.Itinerary{in})[0]
	if got.RealtimeDelta != 190 || got.Legs[3].Start != at(1690) || got.End != at(2350) {
		t.Fatalf("정보 없음 → delta 그대로: delta=%v 둘째=%s end=%s", got.RealtimeDelta, got.Legs[3].Start, got.End)
	}
}

// 버스가 낀 환승(버스 → 지하철)은 여유(0초)를 넘겨도 지연만큼만 민다.
func TestBusTransferDelayStaysLinear(t *testing.T) {
	var calls int32
	srv := fakeBus(t, &calls)
	defer srv.Close()
	c := &Corrector{Bus: &BusClient{Key: "k", HTTP: srv.Client(), Base: srv.URL}, Now: func() time.Time { return now }}
	legs := []otp.Leg{
		{Mode: "WALK", Duration: 300, Start: at(300), End: at(600)},
		{Mode: "BUS", Route: "402", RouteID: "seoul:B_100100063", FromStopID: "seoul:BS_1", TransitLeg: true,
			Start: at(600), End: at(1800)},
		{Mode: "WALK", Duration: 120, Start: at(1800), End: at(1920)},
		{Mode: "SUBWAY", Route: "서울3호선", RouteID: "seoul:RR_3", TransitLeg: true, Start: at(2040), End: at(2640),
			NextDepartures: []string{at(2340)}},
		{Mode: "WALK", Duration: 60, Start: at(2640), End: at(2700)},
	}
	in := otp.Itinerary{Start: at(300), End: at(2700), Duration: 2400, Transfers: 1, Legs: legs}
	got := c.Adjust(context.Background(), []otp.Itinerary{in})[0]
	if got.RealtimeDelta != 60 || got.Legs[3].Start != at(2100) || got.End != at(2760) {
		t.Fatalf("버스 환승은 +60 그대로: delta=%v 둘째=%s end=%s", got.RealtimeDelta, got.Legs[3].Start, got.End)
	}
}

// 연쇄: 첫 환승을 놓쳐 둘째 열차가 +300 이 돼도, 둘째 환승 여유(480초) 안이면 셋째 열차·도착은 그대로.
func TestSubwayChainSecondTransferKept(t *testing.T) {
	in := subwayTransferItinerary()
	in.Legs[4] = otp.Leg{Mode: "WALK", Duration: 60, Start: at(2100), End: at(2160)}
	in.Legs = append(in.Legs,
		otp.Leg{Mode: "SUBWAY", Route: "서울5호선", RouteID: "seoul:RR_5", TransitLeg: true, Start: at(2640), End: at(3240),
			NextDepartures: []string{at(2940)}},
		otp.Leg{Mode: "WALK", Duration: 60, Start: at(3240), End: at(3300)})
	in.End, in.Duration, in.Transfers = at(3300), 2760, 2
	got := line4Arrival(t, 790).Adjust(context.Background(), []otp.Itinerary{in})[0]
	if got.RealtimeDelta != 0 || got.End != at(3300) || got.Legs[3].Start != at(1800) || got.Legs[4].End != at(2460) ||
		got.Legs[5].Start != at(2640) {
		t.Fatalf("둘째 환승은 여유 안: delta=%v end=%s 둘째=%s 셋째=%s", got.RealtimeDelta, got.End, got.Legs[3].Start,
			got.Legs[5].Start)
	}
}
