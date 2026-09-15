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

// 환승 2회 버스(시간표 도착 6030초) vs 지하철 직행(도착 5700초, 실시간 없음). 첫 버스 실시간이 시간표보다 17.5분 일러도
// 뒤 환승은 시간표 차라 버스 도착은 그대로여야 하고, 순위는 지하철이 앞이어야 한다(이슈 #52).
func TestRealtimeDoesNotPullLaterTransfersForward(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)
	now := time.Date(2026, 9, 15, 14, 0, 0, 0, kst)
	ts := func(s int) string { return now.Add(time.Duration(s) * time.Second).Format(time.RFC3339) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"msgHeader":{"headerCd":"0","headerMsg":"ok"},"msgBody":{"itemList":[
		 {"stId":"1","arrmsg1":"2분후[1번째 전]","exps1":"120","arrmsg2":"11분후[6번째 전]","exps2":"660"}]}}`))
	}))
	defer srv.Close()
	b := 1710
	bus3 := itin(ts(b-300), ts(b+4320),
		otp.Leg{Mode: "WALK", Duration: 300, Start: ts(b - 300), End: ts(b)},
		otp.Leg{Mode: "BUS", Route: "402", RouteID: "seoul:B_100100063", FromStopID: "seoul:BS_1", TransitLeg: true,
			Start: ts(b), End: ts(b + 1200)},
		otp.Leg{Mode: "WALK", Duration: 120, Start: ts(b + 1200), End: ts(b + 1320)},
		otp.Leg{Mode: "BUS", Route: "B2", RouteID: "seoul:B_2", FromStopID: "seoul:BS_20", TransitLeg: true,
			Start: ts(b + 1440), End: ts(b + 2640)},
		otp.Leg{Mode: "WALK", Duration: 60, Start: ts(b + 2640), End: ts(b + 2700)},
		otp.Leg{Mode: "BUS", Route: "B3", RouteID: "seoul:B_3", FromStopID: "seoul:BS_30", TransitLeg: true,
			Start: ts(b + 2820), End: ts(b + 4020)},
		otp.Leg{Mode: "WALK", Duration: 300, Start: ts(b + 4020), End: ts(b + 4320)})
	bus3.Transfers = 2
	subway := itin(ts(0), ts(5700),
		otp.Leg{Mode: "SUBWAY", Route: "2호선", RouteID: "seoul:RR_2", TransitLeg: true, Start: ts(300), End: ts(5400)})
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) { return []otp.Itinerary{bus3, subway}, nil }}
	c := &realtime.Corrector{Bus: &realtime.BusClient{Key: "k", HTTP: srv.Client(), Base: srv.URL},
		Now: func() time.Time { return now }}
	p := &Planner{OTP: f, Realtime: c, Now: func() time.Time { return now }}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 2 {
		t.Fatalf("err=%v its=%d", err, len(its))
	}
	if its[0].Legs[0].Route != "2호선" {
		t.Errorf("지하철이 1순위여야: 1순위 첫 leg %s total=%v", its[0].Legs[0].Mode, its[0].DepartIn+its[0].Duration)
	}
	for _, it := range its {
		if it.Transfers == 2 && (!it.Realtime || it.RealtimeDelta != 0 || it.DepartIn+it.Duration != 6030 ||
			it.End != ts(6030)) {
			t.Errorf("버스 총 소요는 시간표 도착 그대로여야: realtime=%v delta=%v departIn=%v dur=%v end=%s",
				it.Realtime, it.RealtimeDelta, it.DepartIn, it.Duration, it.End)
		}
	}
}
