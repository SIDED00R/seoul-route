package otp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 역 ID 가 있으면 출발·도착·경유 모두 좌표 대신 stopLocation 으로 나가야 한다(역사 좌표 스냅 우회, 이슈 #12).
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
