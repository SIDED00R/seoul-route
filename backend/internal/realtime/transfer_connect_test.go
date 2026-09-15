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

// 도보(540~600) → 4호선(600~1200) → 통로(1200~1320) → 3호선(1500~2100, 여유 180−120=60초) → 도보(2100~2160).
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

// 첫 열차 +60초 = 환승 여유(60초) 경계 → 둘째 열차·도착은 시간표 그대로, 출발만 +60.
func TestSubwayDelayWithinSlackKeepsConnection(t *testing.T) {
	in := subwayTransferItinerary()
	got := line4Arrival(t, 660).Adjust(context.Background(), []otp.Itinerary{in})[0]
	if !got.Realtime || got.RealtimeDelta != 0 || got.End != in.End || got.Legs[3].Start != in.Legs[3].Start ||
		got.Legs[1].Start != at(660) || got.Legs[2].End != at(1380) || got.Start != at(600) || got.Duration != 1560 {
		t.Fatalf("여유 안 지연은 도착 불변: delta=%v end=%s 둘째=%s start=%s dur=%v", got.RealtimeDelta, got.End,
			got.Legs[3].Start, got.Start, got.Duration)
	}
}

// 첫 열차 +100초 > 여유 60초 → 통로 끝 1420+120=1540 이후 첫 3호선 1800 → 도착 +300(지연 100초보다 더 늦다).
func TestSubwayDelayBeyondSlackTakesNextTrain(t *testing.T) {
	got := line4Arrival(t, 700).Adjust(context.Background(), []otp.Itinerary{subwayTransferItinerary()})[0]
	if got.RealtimeDelta != 300 || got.Legs[3].Start != at(1800) || got.End != at(2460) || got.Legs[2].End != at(1420) ||
		got.Start != at(640) || got.Duration != 1820 {
		t.Fatalf("다음 열차 기준: delta=%v 둘째=%s end=%s dur=%v", got.RealtimeDelta, got.Legs[3].Start, got.End, got.Duration)
	}
}

// 둘째 leg 의 다음 출발 정보가 없으면 지연만큼만 민다.
func TestSubwayNoNextDepartureFallsBackToDelta(t *testing.T) {
	in := subwayTransferItinerary()
	in.Legs[3].NextDepartures = nil
	got := line4Arrival(t, 700).Adjust(context.Background(), []otp.Itinerary{in})[0]
	if got.RealtimeDelta != 100 || got.Legs[3].Start != at(1600) || got.End != at(2260) {
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

// 연쇄: 첫 환승을 놓쳐 둘째 열차가 +300 이 돼도, 둘째 환승 여유(480−120=360초) 안이면 셋째 열차·도착은 그대로.
func TestSubwayChainSecondTransferKept(t *testing.T) {
	in := subwayTransferItinerary()
	in.Legs[4] = otp.Leg{Mode: "WALK", Duration: 60, Start: at(2100), End: at(2160)}
	in.Legs = append(in.Legs,
		otp.Leg{Mode: "SUBWAY", Route: "서울5호선", RouteID: "seoul:RR_5", TransitLeg: true, Start: at(2640), End: at(3240),
			NextDepartures: []string{at(2940)}},
		otp.Leg{Mode: "WALK", Duration: 60, Start: at(3240), End: at(3300)})
	in.End, in.Duration, in.Transfers = at(3300), 2760, 2
	got := line4Arrival(t, 700).Adjust(context.Background(), []otp.Itinerary{in})[0]
	if got.RealtimeDelta != 0 || got.End != at(3300) || got.Legs[3].Start != at(1800) || got.Legs[4].End != at(2460) ||
		got.Legs[5].Start != at(2640) {
		t.Fatalf("둘째 환승은 여유 안: delta=%v end=%s 둘째=%s 셋째=%s", got.RealtimeDelta, got.End, got.Legs[3].Start,
			got.Legs[5].Start)
	}
}
