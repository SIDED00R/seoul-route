package httpapi

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/route"
	"github.com/SIDED00R/seoul-route/backend/internal/speed"
)

// 북쪽으로 v m/s, 5초 간격 n개 샘플 JSON. 위도 1도 ≈ 111,195m. 서울 안 좌표(37.55, 126.97).
func samplesJSON(t0 time.Time, mode string, v float64, n int, acc float64) string {
	var parts []string
	for i := 0; i < n; i++ {
		ts := t0.Add(time.Duration(i) * 5 * time.Second).Format(time.RFC3339)
		parts = append(parts, fmt.Sprintf(`{"ts":%q,"lat":%.8f,"lon":126.97,"accuracy_m":%g,"mode":%q}`,
			ts, 37.55+v*5*float64(i)/111195, acc, mode))
	}
	return `{"samples":[` + strings.Join(parts, ",") + `]}`
}

// trip 발급 → 업로드(재전송 포함) → 종료 → 프로파일 수축 → /routes/plan 에 개인 속도 주입까지 한 배선.
func TestTripTracesAndSpeedLearning(t *testing.T) {
	s, pool := testServer(t)
	h := s.Router()
	sub := "trip-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = $1`, sub) })
	_, out := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, "")
	token := out["token"].(string)
	userID := out["user_id"].(string)
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM speed_profiles WHERE user_id = $1`, userID)
		pool.Exec(context.Background(), `DELETE FROM trips WHERE user_id = $1`, userID)
	})

	// 프로파일 없음 → 사전값
	rr, prof := do(t, h, http.MethodGet, "/users/me/speed", "", token)
	walk := prof["walk"].(map[string]any)
	if rr.Code != 200 || walk["speed_mps"] != route.DefaultWalk || walk["n_trips"] != float64(0) {
		t.Fatalf("초기 프로파일 code=%d body=%v", rr.Code, prof)
	}

	rr, out = do(t, h, http.MethodPost, "/trips", "", token)
	if rr.Code != 201 || out["trip_id"] == nil {
		t.Fatalf("start code=%d body=%v", rr.Code, out)
	}
	tripID := out["trip_id"].(string)
	// 남의 trip / 없는 trip 은 404
	_, other := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`-b"}`, "")
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = $1`, sub+"-b") })
	if rr, _ := do(t, h, http.MethodPost, "/trips/"+tripID+"/traces", samplesJSON(time.Now(), "walk", 1, 2, 5),
		other["token"].(string)); rr.Code != 404 {
		t.Fatalf("남의 trip code=%d", rr.Code)
	}
	if rr, _ := do(t, h, http.MethodPost, "/trips/not-a-uuid/traces", samplesJSON(time.Now(), "walk", 1, 2, 5),
		token); rr.Code != 404 {
		t.Fatalf("잘못된 id code=%d", rr.Code)
	}
	// 서울 밖 좌표·잘못된 mode 는 400
	busan := `{"samples":[{"ts":"2026-09-13T09:00:00Z","lat":35.1,"lon":129.0,"accuracy_m":5,"mode":"walk"}]}`
	if rr, _ := do(t, h, http.MethodPost, "/trips/"+tripID+"/traces", busan, token); rr.Code != 400 {
		t.Fatalf("부산 좌표 code=%d", rr.Code)
	}
	run := `{"samples":[{"ts":"2026-09-13T09:00:00Z","lat":37.55,"lon":126.97,"accuracy_m":5,"mode":"run"}]}`
	if rr, _ := do(t, h, http.MethodPost, "/trips/"+tripID+"/traces", run, token); rr.Code != 400 {
		t.Fatalf("mode run code=%d", rr.Code)
	}
	// 서버 시각보다 MaxClockSkew 넘게 미래인 ts 는 400(30일 삭제가 ts 기준이라 미래 행이 오래 남는다)
	future := samplesJSON(time.Now().Add(MaxClockSkew+time.Minute), "walk", 1, 1, 5)
	if rr, _ := do(t, h, http.MethodPost, "/trips/"+tripID+"/traces", future, token); rr.Code != 400 {
		t.Fatalf("미래 ts code=%d", rr.Code)
	}
	// 걷기 1.6 m/s 30개(29쌍) + 같은 배치 재전송(멱등) + 자전거 8개(7쌍, 표본 부족) + 대중교통 2개
	t0 := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	body := samplesJSON(t0, "walk", 1.6, 30, 8)
	for i := 0; i < 2; i++ {
		rr, out := do(t, h, http.MethodPost, "/trips/"+tripID+"/traces", body, token)
		if rr.Code != 200 || out["accepted"] != float64(30) {
			t.Fatalf("upload %d code=%d body=%v", i, rr.Code, out)
		}
	}
	do(t, h, http.MethodPost, "/trips/"+tripID+"/traces", samplesJSON(t0.Add(time.Hour), "bicycle", 4, 8, 8), token)
	do(t, h, http.MethodPost, "/trips/"+tripID+"/traces", samplesJSON(t0.Add(2*time.Hour), "transit", 15, 2, 8), token)
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM traces WHERE trip_id = $1`, tripID).Scan(&n)
	if n != 40 {
		t.Fatalf("재전송이 중복 저장됐다: traces=%d", n)
	}

	rr, out = do(t, h, http.MethodPost, "/trips/"+tripID+"/end", "", token)
	if rr.Code != 200 {
		t.Fatalf("end code=%d body=%v", rr.Code, out)
	}
	trip := out["trip"].(map[string]any)
	tw := trip["walk"].(map[string]any)
	tb := trip["bicycle"].(map[string]any)
	if tw["used"] != true || tw["pairs"] != float64(29) || math.Abs(tw["speed_mps"].(float64)-1.6) > 0.01 {
		t.Fatalf("trip walk=%v", tw)
	}
	if tb["used"] != false || tb["pairs"] != float64(7) {
		t.Fatalf("trip bicycle(표본 부족)=%v", tb)
	}
	// 프로파일: 걷기 (5×1.2 + 1.6)/6 = 1.2667, 자전거는 사전값 그대로
	pw := out["profile"].(map[string]any)["walk"].(map[string]any)
	v1 := tw["speed_mps"].(float64) // 하버사인 반올림으로 1.6 에서 1e-4 안쪽으로 어긋난다 → 실측값으로 수축을 검산
	if pw["n_trips"] != float64(1) || math.Abs(pw["speed_mps"].(float64)-speed.Shrink(route.DefaultWalk, v1, 1)) > 1e-9 {
		t.Fatalf("profile walk=%v", pw)
	}
	pb := out["profile"].(map[string]any)["bicycle"].(map[string]any)
	if pb["n_trips"] != float64(0) || pb["speed_mps"] != route.DefaultBike {
		t.Fatalf("profile bicycle=%v", pb)
	}
	// 두 번 종료·종료 후 업로드는 409
	if rr, _ := do(t, h, http.MethodPost, "/trips/"+tripID+"/end", "", token); rr.Code != 409 {
		t.Fatalf("재종료 code=%d", rr.Code)
	}
	if rr, _ := do(t, h, http.MethodPost, "/trips/"+tripID+"/traces", body, token); rr.Code != 409 {
		t.Fatalf("종료 후 업로드 code=%d", rr.Code)
	}
	// 두 번째 trip 걷기 1.0 → (6 + 1.6 + 1.0)/7 = 1.2286 (sum 누적, n 증가). 종료를 동시에 두 번 보내도 한 번만 반영된다
	// (앱이 타임아웃 뒤 재시도하면 서버에는 두 요청이 겹친다): 200 과 409 하나씩, n_trips 는 2.
	_, out = do(t, h, http.MethodPost, "/trips", "", token)
	trip2 := out["trip_id"].(string)
	do(t, h, http.MethodPost, "/trips/"+trip2+"/traces", samplesJSON(t0.Add(3*time.Hour), "walk", 1.0, 30, 8), token)
	// 풀을 미리 데운다: 새 커넥션을 여는 약 40ms 동안 첫 요청이 끝나 버리면 두 요청이 겹치지 않아 사전 검사만으로 409 가 난다.
	// 커넥션이 준비돼 있어야 둘 다 ownTrip 을 통과해 조건부 UPDATE 가 갈린다.
	var warm sync.WaitGroup
	for i := 0; i < 4; i++ {
		warm.Add(1)
		go func() {
			defer warm.Done()
			pool.Exec(context.Background(), `SELECT pg_sleep(0.3)`)
		}()
	}
	warm.Wait()
	codes := make(chan int, 2)
	results := make(chan map[string]any, 2)
	for i := 0; i < 2; i++ {
		go func() {
			rr, out := do(t, h, http.MethodPost, "/trips/"+trip2+"/end", "", token)
			codes <- rr.Code
			results <- out
		}()
	}
	c1, c2 := <-codes, <-codes
	r1, r2 := <-results, <-results
	if c1+c2 != 200+409 {
		t.Fatalf("동시 종료 code=%d,%d (200+409 여야)", c1, c2)
	}
	rr, out = &httptest.ResponseRecorder{Code: 200}, r1
	if c1 != 200 {
		out = r2
	}
	pw = out["profile"].(map[string]any)["walk"].(map[string]any)
	v2 := out["trip"].(map[string]any)["walk"].(map[string]any)["speed_mps"].(float64)
	want2 := speed.Shrink(route.DefaultWalk, v1+v2, 2)
	if rr.Code != 200 || pw["n_trips"] != float64(2) || math.Abs(pw["speed_mps"].(float64)-want2) > 1e-9 {
		t.Fatalf("2번째 trip 후 profile walk=%v", pw)
	}
	// /routes/plan 이 학습된 걷기 속도를 OTP 요청에 넣는다(Planner 는 요청 속도를 그대로 응답 walk_speed 로 돌려준다)
	s.Planner = &route.Planner{OTP: &deadlineOTP{}}
	rr, out = do(t, h, http.MethodPost, "/routes/plan",
		`{"origin":{"lat":37.5547,"lon":126.9707},"destination":{"lat":37.4979,"lon":127.0276}}`, token)
	if rr.Code != 200 || math.Abs(out["walk_speed"].(float64)-want2) > 1e-9 || out["bike_speed"] != route.DefaultBike {
		t.Fatalf("plan 속도 주입 code=%d walk=%v bike=%v", rr.Code, out["walk_speed"], out["bike_speed"])
	}
	// 30일 지난 궤적 삭제
	pool.Exec(context.Background(), `UPDATE traces SET ts = ts - interval '40 days' WHERE trip_id = $1`, tripID)
	if deleted, err := speed.PurgeOldTraces(context.Background(), pool, time.Now()); err != nil || deleted < 40 {
		t.Fatalf("purge deleted=%d err=%v", deleted, err)
	}
	// 탈퇴하면 trip·궤적·프로파일이 사라진다
	if rr, _ := do(t, h, http.MethodDelete, "/users/me", "", token); rr.Code != 204 {
		t.Fatalf("delete code=%d", rr.Code)
	}
	pool.QueryRow(context.Background(), `SELECT count(*) FROM traces WHERE trip_id IN ($1, $2)`, tripID, trip2).Scan(&n)
	if n != 0 {
		t.Fatalf("탈퇴 후 traces=%d", n)
	}
}
