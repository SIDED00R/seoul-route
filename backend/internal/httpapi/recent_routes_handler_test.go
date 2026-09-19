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

// 탈퇴가 끝난 뒤에 도착한 검색은 이동 기록을 되살리면 안 된다. 탈퇴는 소프트 삭제라 FK 로는 막히지 않고,
// 그 계정 토큰은 이미 401 이라 사용자가 지울 수도 없다.
func TestRecentRoutesNotSavedForDeletedUser(t *testing.T) {
	s, pool := testServer(t)
	s.Planner = &route.Planner{OTP: okOTP{}}
	h := s.Router()
	sub := "sub-deleted-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = $1`, sub) })
	do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, "")

	var userID string
	if err := pool.QueryRow(context.Background(),
		`UPDATE users SET deleted_at = now() WHERE google_sub = $1 RETURNING id`, sub).Scan(&userID); err != nil {
		t.Fatalf("탈퇴 처리: %v", err)
	}
	// 탐색 요청은 탈퇴 전에 인증을 통과했다고 보고, 저장 경로만 직접 부른다.
	s.saveRecentRoute(context.Background(), userID, route.PlanRequest{
		Origin:      route.Point{Lat: 37.5547, Lon: 126.9707, Name: "서울역"},
		Destination: route.Point{Lat: 37.4979, Lon: 127.0276, Name: "강남역"},
	})
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM recent_routes WHERE user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("조회: %v", err)
	}
	if n != 0 {
		t.Errorf("탈퇴한 사용자에게 최근 경로가 %d줄 저장됐다", n)
	}
}

// 탈퇴가 진행 중일 때 들어온 검색은 기다렸다가 아무것도 남기지 않아야 한다. 사용자 행을 두 흐름이 같은 순서로
// 잠그므로, 탈퇴가 먼저 잡으면 저장은 그 뒤에 "이미 탈퇴" 로 판정한다.
func TestRecentRoutesLosesRaceWithDelete(t *testing.T) {
	s, pool := testServer(t)
	ctx := context.Background()
	sub := "sub-race-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM users WHERE google_sub = $1`, sub) })
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users(google_sub) VALUES ($1) RETURNING id`, sub).Scan(&userID); err != nil {
		t.Fatalf("사용자 생성: %v", err)
	}

	// 탈퇴 트랜잭션이 사용자 행을 먼저 잠근다(handleDeleteMe 와 같은 순서).
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) // 실패해도 잠금을 놓아 저장 쪽이 영영 기다리지 않게
	if _, err := tx.Exec(ctx, `UPDATE users SET deleted_at = now() WHERE id = $1`, userID); err != nil {
		t.Fatalf("탈퇴 UPDATE: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.saveRecentRoute(ctx, userID, route.PlanRequest{
			Origin:      route.Point{Lat: 37.5547, Lon: 126.9707, Name: "서울역"},
			Destination: route.Point{Lat: 37.4979, Lon: 127.0276, Name: "강남역"},
		})
	}()
	waited := true
	select { // 저장이 잠금에 걸려 기다리는지 확인한다
	case <-done:
		waited = false
	case <-time.After(200 * time.Millisecond):
	}

	if _, err := tx.Exec(ctx, `DELETE FROM recent_routes WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("기록 삭제: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	<-done
	if !waited {
		t.Error("탈퇴가 잠근 행을 기다리지 않고 저장이 끝났다")
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM recent_routes WHERE user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("조회: %v", err)
	}
	if n != 0 {
		t.Errorf("탈퇴 뒤에 최근 경로가 %d줄 남았다", n)
	}
}
