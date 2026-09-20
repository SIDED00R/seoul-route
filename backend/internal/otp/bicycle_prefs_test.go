package otp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// capturePlan 은 Plan 이 보낸 GraphQL 변수를 돌려준다.
func capturePlan(t *testing.T, r Request) map[string]any {
	t.Helper()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		json.Unmarshal(body, &got)
		w.Write([]byte(`{"data":{"planConnection":{"routingErrors":[],"edges":[]}}}`))
	}))
	defer srv.Close()
	c := &Client{URL: srv.URL, HTTP: srv.Client()}
	if _, err := c.Plan(context.Background(), r); err != ErrNoRoute {
		t.Fatalf("빈 edges 는 ErrNoRoute 여야: %v", err)
	}
	if got == nil {
		t.Fatal("요청 본문을 못 읽었다")
	}
	return got["variables"].(map[string]any)
}

func bicycleOf(t *testing.T, vars map[string]any) map[string]any {
	t.Helper()
	prefs, ok := vars["preferences"].(map[string]any)
	if !ok {
		t.Fatalf("preferences 가 없다: %v", vars)
	}
	b, ok := prefs["street"].(map[string]any)["bicycle"].(map[string]any)
	if !ok {
		t.Fatalf("bicycle 선호가 없다: %v", prefs)
	}
	return b
}

// optimizationOf 는 자전거 최적화 기준. 빠져 있으면 빈 문자열이다.
func optimizationOf(b map[string]any) any {
	o, ok := b["optimization"].(map[string]any)
	if !ok {
		return ""
	}
	return o["type"]
}

// 자전거 최적화 기준과 끌기 비용은 요청마다 나가야 한다. OTP 기본값(SAFE_STREETS·끌기 5.0)이면 자전거길을
// 우대하고 끌어야 지나가는 구간을 피해 크게 돌아간다.
func TestPlanSendsBicycleOptimizationAndWalkCost(t *testing.T) {
	vars := capturePlan(t, Request{
		Origin: Coord{37.55, 126.97}, Destination: Coord{37.49, 127.02},
		Modes: Modes{Direct: []string{"BICYCLE_RENTAL", "WALK"}, Only: true}, BikeSpeed: 4.2, First: 3,
	})
	b := bicycleOf(t, vars)
	if got := optimizationOf(b); got != "SHORTEST_DURATION" {
		t.Fatalf("최적화 기준 %v", got)
	}
	rel := b["walk"].(map[string]any)["cost"].(map[string]any)["reluctance"]
	if rel != BikeWalkReluctance {
		t.Fatalf("끌기 비용 %v (원하는 값 %v)", rel, BikeWalkReluctance)
	}
	// 2.0 을 넘으면 한강 다리 보도를 끌고 건너는 대신 다른 다리로 돌아간다(docs/bicycle-routing.md 실측).
	if BikeWalkReluctance > 2.0 {
		t.Fatalf("끌기 비용 %v: 2.0 을 넘으면 다리를 안 건넌다", BikeWalkReluctance)
	}
	if b["speed"] != 4.2 {
		t.Fatalf("자전거 속도 %v", b["speed"])
	}
	walk := vars["preferences"].(map[string]any)["street"].(map[string]any)["walk"]
	if walk != nil {
		t.Fatalf("걷기 속도를 안 줬는데 walk 선호가 나갔다: %v", walk)
	}
}

// 속도를 안 주면 그 항목만 빠지고 최적화 기준·끌기 비용은 그대로 나간다(OTP 기본 속도를 쓰는 경로).
func TestPlanOmitsBicycleSpeedWhenUnset(t *testing.T) {
	vars := capturePlan(t, Request{
		Origin: Coord{37.55, 126.97}, Destination: Coord{37.49, 127.02},
		Modes: Modes{Direct: []string{"WALK"}, Only: true}, WalkSpeed: 1.3, First: 3,
	})
	b := bicycleOf(t, vars)
	if _, ok := b["speed"]; ok {
		t.Fatalf("속도 0 인데 speed 가 나갔다: %v", b)
	}
	if got := optimizationOf(b); got != "SHORTEST_DURATION" {
		t.Fatalf("최적화 기준 %v", got)
	}
	walk := vars["preferences"].(map[string]any)["street"].(map[string]any)["walk"].(map[string]any)
	if walk["speed"] != 1.3 {
		t.Fatalf("걷기 속도 %v", walk["speed"])
	}
}
