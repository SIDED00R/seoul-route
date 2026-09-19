package otp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 앞뒤 차는 leg 출발 ±3시간 안·같은 시각 제외(배차 기반 trip 의 막차 sentinel 01:01 제거), 그리고 소요시간이
// 현재 leg 의 0.5~2배인 것만(2호선 사당→강남 9분 구간에 반대 방향으로 한 바퀴 도는 81분 열차가 섞이던 실측).
func TestNearbyDepartures(t *testing.T) {
	mk := func(s string, dur float64) legTime {
		var l legTime
		l.Start.ScheduledTime = s
		l.Duration = dur
		return l
	}
	got := nearbyDepartures([]legTime{
		mk("2026-09-14T14:05:00+09:00", 540),  // 같은 방향
		mk("2026-09-14T14:03:00+09:00", 4860), // 반대 방향(한 바퀴)
		mk("2026-09-14T14:07:00+09:00", 330),  // 1호선 계열 노선처럼 조금 다른 소요는 유지
		mk("2026-09-15T01:01:00+09:00", 540),  // 막차 sentinel
		mk("2026-09-14T14:00:00+09:00", 540),  // 자기 자신
		mk("bad", 540),
	}, "2026-09-14T14:00:00+09:00", 540)
	if len(got) != 2 || got[0] != "2026-09-14T14:05:00+09:00" || got[1] != "2026-09-14T14:07:00+09:00" {
		t.Fatalf("±3시간·소요 0.5~2배·자기 자신 제외: %v", got)
	}
	got = nearbyDepartures([]legTime{mk("2026-09-14T14:05:00+09:00", 4860)}, "2026-09-14T14:00:00+09:00", 0)
	if len(got) != 1 {
		t.Fatalf("기준 소요가 없으면 소요 조건은 걸지 않는다: %v", got)
	}
}

// 역 ID 가 있으면 출발·도착·경유 모두 좌표 대신 stopLocation 으로 나가야 한다(역사 좌표 스냅 우회).
func TestPlanSendsStopLocations(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
		w.Write([]byte(`{"data":{"planConnection":{"routingErrors":[],"edges":[]}}}`))
	}))
	defer srv.Close()
	c := &Client{URL: srv.URL, HTTP: srv.Client()}
	_, err := c.Plan(context.Background(), Request{
		Origin: Coord{37.55, 126.97}, Destination: Coord{37.49, 127.02}, OriginStop: "seoul:ST_서울",
		Via: []Coord{{37.52, 126.92}, {37.50, 127.00}}, ViaStops: []string{"seoul:ST_여의도", ""},
		Modes: Modes{Direct: []string{"WALK"}, Only: true}, First: 3,
	})
	if err != ErrNoRoute {
		t.Fatalf("빈 edges 는 ErrNoRoute 여야: %v", err)
	}
	vars := got["variables"].(map[string]any)
	origin := vars["origin"].(map[string]any)["location"].(map[string]any)
	if origin["stopLocation"].(map[string]any)["stopLocationId"] != "seoul:ST_서울" || origin["coordinate"] != nil {
		t.Fatalf("origin 은 stopLocation 만: %v", origin)
	}
	dest := vars["destination"].(map[string]any)["location"].(map[string]any)
	if dest["coordinate"] == nil || dest["stopLocation"] != nil {
		t.Fatalf("역 ID 없는 destination 은 좌표: %v", dest)
	}
	via := vars["via"].([]any)
	v0 := via[0].(map[string]any)["visit"].(map[string]any)
	v1 := via[1].(map[string]any)["visit"].(map[string]any)
	ids, ok := v0["stopLocationIds"].([]any)
	if !ok || len(ids) != 1 || ids[0] != "seoul:ST_여의도" || v0["coordinate"] != nil {
		t.Fatalf("경유 1 은 stopLocationIds: %v", v0)
	}
	if v1["coordinate"] == nil || v1["stopLocationIds"] != nil {
		t.Fatalf("역 ID 없는 경유 2 는 좌표: %v", v1)
	}
}
