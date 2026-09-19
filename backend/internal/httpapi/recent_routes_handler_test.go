package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

type okOTP struct{}

func (okOTP) Plan(context.Context, otp.Request) ([]otp.Itinerary, error) {
	return []otp.Itinerary{{Start: "2026-09-19T09:00:00+09:00", End: "2026-09-19T09:30:00+09:00", Duration: 1800,
		Legs: []otp.Leg{{Mode: "WALK"}}}}, nil
}

// 검색하면 최근 경로에 쌓이고, 같은 출발·도착은 한 줄로 묶이며, 새 것부터 나온다.
func TestRecentRoutesRecordAndList(t *testing.T) {
	s, pool := testServer(t)
	s.Planner = &route.Planner{OTP: okOTP{}}
	h := s.Router()
	sub := "sub-recent-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = $1`, sub) })
	_, out := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, "")
	token := out["token"].(string)

	plan := func(body string) {
		t.Helper()
		if rr, out := do(t, h, http.MethodPost, "/routes/plan", body, token); rr.Code != 200 {
			t.Fatalf("plan code=%d body=%v", rr.Code, out)
		}
	}
	seoulToGangnam := `{"origin":{"lat":37.5547,"lon":126.9707,"name":"서울역"},` +
		`"destination":{"lat":37.4979,"lon":127.0276,"name":"강남역"}}`
	plan(seoulToGangnam)
	plan(`{"origin":{"lat":37.5547,"lon":126.9707,"name":"서울역"},` +
		`"destination":{"lat":37.5326,"lon":126.9906,"name":"이태원"}}`)
	plan(seoulToGangnam) // 같은 검색 → 줄이 늘지 않고 맨 위로 온다

	rr, out := do(t, h, http.MethodGet, "/routes/recent", "", token)
	if rr.Code != 200 {
		t.Fatalf("code=%d", rr.Code)
	}
	list, _ := out["routes"].([]any)
	if len(list) != 2 {
		t.Fatalf("줄 수=%d body=%v", len(list), out)
	}
	first, _ := list[0].(map[string]any)["request"].(map[string]any)
	dest, _ := first["destination"].(map[string]any)
	if dest["name"] != "강남역" {
		t.Errorf("가장 최근이 맨 위여야 한다: %v", first)
	}
	// 저장한 요청을 그대로 다시 보내면 같은 검색이 된다.
	again, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if rr, _ := do(t, h, http.MethodPost, "/routes/plan", string(again), token); rr.Code != 200 {
		t.Errorf("저장된 요청 재사용: code=%d", rr.Code)
	}

	if rr, _ := do(t, h, http.MethodDelete, "/routes/recent", "", token); rr.Code != 204 {
		t.Fatalf("삭제 code=%d", rr.Code)
	}
	_, out = do(t, h, http.MethodGet, "/routes/recent", "", token)
	if list, _ := out["routes"].([]any); len(list) != 0 {
		t.Errorf("삭제 뒤: %v", out)
	}
}

// 경유지·구간 수단이 다르면 다른 줄이다(고른 대로 다시 검색되어야 한다).
func TestRecentRoutesKeyIncludesViaAndModes(t *testing.T) {
	p := func(lat, lon float64) route.Point { return route.Point{Lat: lat, Lon: lon} }
	base := route.PlanRequest{Origin: p(37.5547, 126.9707), Destination: p(37.4979, 127.0276)}
	withVia := base
	withVia.Via = []route.Point{p(37.52, 126.92)}
	withModes := base
	withModes.Modes = []route.SegmentMode{route.ModeBike}
	moved := route.PlanRequest{Origin: p(37.5547, 126.9707), Destination: p(37.4979, 127.0299)}
	sameSpot := route.PlanRequest{Origin: p(37.55470004, 126.97070004), Destination: p(37.4979, 127.0276)}
	keys := map[string]string{
		"기본": recentRouteKey(base), "경유지": recentRouteKey(withVia), "수단 고정": recentRouteKey(withModes),
		"도착지 다름": recentRouteKey(moved),
	}
	seen := map[string]string{}
	for name, k := range keys {
		if other, dup := seen[k]; dup {
			t.Errorf("%s 와 %s 가 같은 줄로 묶인다", name, other)
		}
		seen[k] = name
	}
	if recentRouteKey(sameSpot) != recentRouteKey(base) { // 1m 안쪽 차이는 같은 곳
		t.Errorf("소수 5자리 아래 차이는 같은 줄이어야 한다")
	}
}
