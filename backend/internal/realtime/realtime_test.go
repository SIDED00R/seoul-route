package realtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

var now = time.Date(2026, 9, 14, 14, 0, 0, 0, time.FixedZone("KST", 9*3600))

func at(sec int) string { return now.Add(time.Duration(sec) * time.Second).Format(time.RFC3339) }

// 가짜 서울 버스 도착정보: 402 노선, 정류장 BS_1 은 첫 차 120초·둘째 차 660초, BS_2 는 출발대기(예측 없음).
func fakeBus(t *testing.T, calls *int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		if r.URL.Query().Get("busRouteId") != "100100063" || r.URL.Query().Get("serviceKey") != "k" {
			t.Errorf("bad query %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"msgHeader":{"headerCd":"0","headerMsg":"ok"},"msgBody":{"itemList":[
		 {"stId":"1","arrmsg1":"2분후[3번째 전]","exps1":"120","arrmsg2":"11분후[8번째 전]","exps2":"660"},
		 {"stId":"2","arrmsg1":"출발대기","exps1":"0","arrmsg2":"출발대기","exps2":"0"}]}}`))
	}))
}

func fakeSubway(t *testing.T, calls *int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		if !strings.Contains(r.URL.Path, "/api/subway/s/json/realtimeStationArrival/") {
			t.Errorf("bad path %s", r.URL.Path)
		}
		w.Write([]byte(`{"errorMessage":{"status":200,"code":"INFO-000","message":"ok"},"realtimeArrivalList":[
		 {"subwayId":"1004","trainLineNm":"불암산행 - 회현방면","barvlDt":"180","arvlCd":"99"},
		 {"subwayId":"1004","trainLineNm":"오이도행 - 숙대입구방면","barvlDt":"60","arvlCd":"99"},
		 {"subwayId":"1001","trainLineNm":"광운대행 - 시청방면","barvlDt":"0","arvlCd":"99"}]}`))
	}))
}

func itinerary(access float64, leg otp.Leg) otp.Itinerary {
	board := int(access) + 9*60 // 시간표상 탑승 = 접근 후 9분 대기
	leg.Start, leg.End, leg.TransitLeg = at(board), at(board+1200), true
	return otp.Itinerary{Start: at(0), End: at(board + 1200), Duration: float64(board + 1200),
		Legs: []otp.Leg{{Mode: "WALK", Duration: access, Start: at(0), End: at(int(access))}, leg}}
}

func TestBusFirstBoardingUsesRealtime(t *testing.T) {
	var calls int32
	srv := fakeBus(t, &calls)
	defer srv.Close()
	c := &Corrector{Bus: &BusClient{Key: "k", HTTP: srv.Client(), Base: srv.URL}, Now: func() time.Time { return now }}
	bus := otp.Leg{Mode: "BUS", Route: "402", RouteID: "seoul:B_100100063", FromStopID: "seoul:BS_1"}
	// 정류장까지 5분 → 2분 뒤 차는 놓치고 11분 뒤 차를 탄다. 시간표(14분)보다 3분 빠르므로 delta = -180.
	// 출발은 now 보다 앞설 수 없으니 그대로(정류장 대기가 3분 줄어든다) → 소요 2040−180 = 1860.
	out := c.Adjust(context.Background(), []otp.Itinerary{itinerary(300, bus)})
	got := out[0]
	if !got.Realtime || got.RealtimeDelta != -180 || got.Duration != 1860 || got.Start != at(0) {
		t.Fatalf("보정: %+v", got)
	}
	if got.Legs[1].Start != at(660) || got.End != at(660+1200) || got.Legs[0].Start != at(0) {
		t.Fatalf("탑승 leg·종료는 delta 만큼, 접근 도보는 그대로: %+v", got)
	}
	if a := got.Legs[1].RealtimeArrivals; len(a) != 2 || a[0] != 120 || a[1] != 660 {
		t.Fatalf("첫 탑승 leg 에 실시간 다음 차 목록(놓친 차 포함): %v", a)
	}
	// 예측 없는 정류장은 시간표 그대로. 같은 노선은 캐시라 HTTP 1회.
	bus2 := bus
	bus2.FromStopID = "seoul:BS_2"
	out = c.Adjust(context.Background(), []otp.Itinerary{itinerary(300, bus2)})
	if out[0].Realtime || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("예측 없음 → 무보정, 캐시 → 콜 1회: %+v calls=%d", out[0], calls)
	}
}

func TestSubwayFirstBoardingMatchesLineAndDirection(t *testing.T) {
	var calls int32
	srv := fakeSubway(t, &calls)
	defer srv.Close()
	c := &Corrector{Subway: &SubwayClient{Key: "s", HTTP: srv.Client(), Base: srv.URL},
		Now: func() time.Time { return now }}
	sub := otp.Leg{Mode: "SUBWAY", Route: "서울4호선", RouteID: "seoul:RR_4", FromName: "서울(4호선)",
		NextStop: "회현(4호선)"}
	// 접근 1분, 회현방면 4호선 열차 180초 → 시간표(10분)보다 7분 빠름.
	out := c.Adjust(context.Background(), []otp.Itinerary{itinerary(60, sub)})
	if !out[0].Realtime || out[0].RealtimeDelta != -420 {
		t.Fatalf("지하철 보정: %+v", out[0])
	}
	// 1호선(barvlDt 0, 진입 전) 은 예측 없음 → 무보정.
	line1 := otp.Leg{Mode: "SUBWAY", Route: "서울1호선", FromName: "서울(1호선)", NextStop: "시청(1호선)"}
	out = c.Adjust(context.Background(), []otp.Itinerary{itinerary(60, line1)})
	if out[0].Realtime || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("미정 열차는 무보정, 같은 역은 캐시: %+v calls=%d", out[0], calls)
	}
}

// 실시간 차가 시간표보다 늦으면 출발도 그만큼 늦춘다(소요시간은 그대로, 출발 대기만 늘어난다).
func TestLaterBusShiftsDeparture(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(`{"msgHeader":{"headerCd":"0","headerMsg":"ok"},"msgBody":{"itemList":[
		 {"stId":"1","arrmsg1":"15분후[9번째 전]","exps1":"900","arrmsg2":"출발대기","exps2":"0"}]}}`))
	}))
	defer srv.Close()
	c := &Corrector{Bus: &BusClient{Key: "k", HTTP: srv.Client(), Base: srv.URL}, Now: func() time.Time { return now }}
	bus := otp.Leg{Mode: "BUS", RouteID: "seoul:B_1", FromStopID: "seoul:BS_1"}
	out := c.Adjust(context.Background(), []otp.Itinerary{itinerary(60, bus)}) // 시간표 탑승 600초, 실시간 900초
	got := out[0]
	if got.RealtimeDelta != 300 || got.Start != at(300) || got.Legs[0].Start != at(300) || got.Legs[1].Start != at(900) {
		t.Fatalf("출발·접근·탑승 전부 +300초: %+v", got)
	}
	if got.Duration != 600+1200 || got.End != at(900+1200) {
		t.Fatalf("소요시간은 그대로: %+v", got)
	}
}

// 캐시된 상대 ETA 는 조회 시각 기준이다. 30초 안에 다시 쓰면 경과시간을 빼야 같은 차를 같은 절대 시각으로 본다.
// (Corrector.Now 를 주입하지 않아 실제 벽시계로 잰다 — 차감도 벽시계 기준이라 운영과 같은 시계다.)
func TestCachedETASubtractsElapsed(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(`{"msgHeader":{"headerCd":"0","headerMsg":"ok"},"msgBody":{"itemList":[
		 {"stId":"1","arrmsg1":"10분후[6번째 전]","exps1":"600","arrmsg2":"출발대기","exps2":"0"}]}}`))
	}))
	defer srv.Close()
	c := &Corrector{Bus: &BusClient{Key: "k", HTTP: srv.Client(), Base: srv.URL}}
	bus := otp.Leg{Mode: "BUS", RouteID: "seoul:B_1", FromStopID: "seoul:BS_1"}
	mk := func() otp.Itinerary {
		n := time.Now()
		it := otp.Itinerary{Start: n.Format(time.RFC3339), End: n.Add(30 * time.Minute).Format(time.RFC3339), Duration: 1800}
		it.Legs = []otp.Leg{{Mode: "WALK", Duration: 60, Start: it.Start, End: n.Add(time.Minute).Format(time.RFC3339)}}
		b := bus
		b.Start, b.End, b.TransitLeg = n.Add(5*time.Minute).Format(time.RFC3339), it.End, true
		it.Legs = append(it.Legs, b)
		return it
	}
	first := c.Adjust(context.Background(), []otp.Itinerary{mk()})[0]
	time.Sleep(1500 * time.Millisecond)
	second := c.Adjust(context.Background(), []otp.Itinerary{mk()})[0]
	b1, _ := time.Parse(time.RFC3339, first.Legs[1].Start)
	b2, _ := time.Parse(time.RFC3339, second.Legs[1].Start)
	if atomic.LoadInt32(&calls) != 1 || !first.Realtime || !second.Realtime {
		t.Fatalf("캐시 재사용·보정: calls=%d %v %v", calls, first.Realtime, second.Realtime)
	}
	if d := b2.Sub(b1); d < -time.Second || d > time.Second {
		t.Fatalf("같은 캐시 항목의 탑승 절대 시각이 흘러가면 안 됨: %v → %v (차이 %v)", b1, b2, d)
	}
}

func TestNoClientsOrNoTransitLeavesItineraryUnchanged(t *testing.T) {
	c := &Corrector{Now: func() time.Time { return now }}
	walk := otp.Itinerary{Duration: 600, Legs: []otp.Leg{{Mode: "WALK", Duration: 600}}}
	bus := itinerary(60, otp.Leg{Mode: "BUS", RouteID: "seoul:B_1", FromStopID: "seoul:BS_1"})
	out := c.Adjust(context.Background(), []otp.Itinerary{walk, bus})
	if out[0].Realtime || out[1].Realtime || out[1].Duration != bus.Duration {
		t.Fatalf("클라이언트 없으면 그대로: %+v", out)
	}
}

func TestBusETAAndNextStop(t *testing.T) {
	if s, ok := busETA("곧 도착", "0"); !ok || s != 0 {
		t.Fatal("곧 도착은 0초")
	}
	if _, ok := busETA("운행종료", "0"); ok {
		t.Fatal("운행종료는 예측 없음")
	}
	if nextStopOf("인천공항2터미널행 - 공덕방면 (급행) (막차)") != "공덕" || nextStopOf("서울") != "" {
		t.Fatal("방면 파싱")
	}
	if SubwayIDFor("서울4호선") != "1004" || SubwayIDFor("경의중앙선") != "1063" || SubwayIDFor("KTX") != "" {
		t.Fatal("subwayId 매핑")
	}
}
