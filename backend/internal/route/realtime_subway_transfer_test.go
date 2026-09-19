package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
	"github.com/SIDED00R/seoul-route/backend/internal/realtime"
)

// 지하철 환승 후보의 첫 열차가 환승 여유를 넘게 늦으면 둘째 열차는 다음 차 기준이라 도착이 지연(190초)보다 더 늦고(+300),
// 환승 없는 철도 직행(실시간 없음) 뒤로 밀린다.
func TestRealtimeMissedSubwayTransferReranks(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)
	now := time.Date(2026, 9, 15, 14, 0, 0, 0, kst)
	ts := func(s int) string { return now.Add(time.Duration(s) * time.Second).Format(time.RFC3339) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errorMessage":{"status":200,"code":"INFO-000","message":"ok"},"realtimeArrivalList":[
		 {"subwayId":"1004","trainLineNm":"불암산행 - 회현방면","barvlDt":"790","arvlCd":"99"}]}`))
	}))
	defer srv.Close()
	transfer := itin(ts(540), ts(2160),
		otp.Leg{Mode: "WALK", Duration: 60, Start: ts(540), End: ts(600)},
		otp.Leg{Mode: "SUBWAY", Route: "서울4호선", RouteID: "seoul:RR_4", FromName: "서울(4호선)", NextStop: "회현(4호선)",
			TransitLeg: true, Start: ts(600), End: ts(1200)},
		otp.Leg{Mode: "WALK", Duration: 120, Start: ts(1200), End: ts(1320)},
		otp.Leg{Mode: "SUBWAY", Route: "서울3호선", RouteID: "seoul:RR_3", FromName: "충무로(3호선)", TransitLeg: true,
			Start: ts(1500), End: ts(2100), NextDepartures: []string{ts(1800)}},
		otp.Leg{Mode: "WALK", Duration: 60, Start: ts(2100), End: ts(2160)})
	transfer.Transfers = 1
	direct := itin(ts(0), ts(2600),
		otp.Leg{Mode: "RAIL", Route: "경부선", RouteID: "seoul:K_1", TransitLeg: true, Start: ts(300), End: ts(2600)})
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) { return []otp.Itinerary{transfer, direct}, nil }}
	c := &realtime.Corrector{Subway: &realtime.SubwayClient{Key: "s", HTTP: srv.Client(), Base: srv.URL},
		Now: func() time.Time { return now }}
	p := &Planner{OTP: f, Realtime: c, Now: func() time.Time { return now }}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 2 {
		t.Fatalf("err=%v its=%d", err, len(its))
	}
	if its[0].Legs[0].Route != "경부선" || its[1].RealtimeDelta != 300 || its[1].End != ts(2460) {
		t.Fatalf("직행이 1순위, 환승 후보는 도착 +300: 1순위=%s 환승 delta=%v end=%s", its[0].Legs[0].Route,
			its[1].RealtimeDelta, its[1].End)
	}
}
