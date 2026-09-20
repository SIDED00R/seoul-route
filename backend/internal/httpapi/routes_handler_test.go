package httpapi

import (
	"context"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

// deadlineOTP 는 핸들러가 넘긴 컨텍스트의 데드라인을 기록한다.
type deadlineOTP struct{ remaining time.Duration }

func (d *deadlineOTP) Plan(ctx context.Context, _ otp.Request) ([]otp.Itinerary, error) {
	if dl, ok := ctx.Deadline(); ok {
		d.remaining = time.Until(dl)
	}
	return []otp.Itinerary{{Start: "2026-09-14T14:00:00+09:00", End: "2026-09-14T14:30:00+09:00", Duration: 1800,
		Legs: []otp.Leg{{Mode: "WALK"}}}}, nil
}

// /routes/plan 은 전역 30초가 아니라 PlanTimeout(60초) 예산을 받아야 한다. 부산 좌표는 400.
func TestPlanRouteTimeoutBudgetAndValidation(t *testing.T) {
	s, pool := testServer(t)
	d := &deadlineOTP{}
	s.Planner = &route.Planner{OTP: d}
	h := s.Router()
	sub := "sub-plan-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = $1`, sub) })
	_, out := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, "")
	token := out["token"].(string)

	rr, out := do(t, h, http.MethodPost, "/routes/plan",
		`{"origin":{"lat":37.5547,"lon":126.9707},"destination":{"lat":37.4979,"lon":127.0276}}`, token)
	if rr.Code != 200 || out["walk_speed"] != 1.2 {
		t.Fatalf("code=%d body=%v", rr.Code, out)
	}
	if d.remaining <= DefaultTimeout+10*time.Second {
		t.Fatalf("핸들러 데드라인 남은 시간 %v: 전역 30초에 갇혀 있다", d.remaining)
	}
	rr, _ = do(t, h, http.MethodPost, "/routes/plan",
		`{"origin":{"lat":37.5547,"lon":126.9707},"destination":{"lat":35.1,"lon":129.0}}`, token)
	if rr.Code != 400 {
		t.Fatalf("부산 좌표 code=%d", rr.Code)
	}
	if rr, _ := do(t, h, http.MethodPost, "/routes/plan", `{}`, ""); rr.Code != 401 {
		t.Fatalf("미인증 code=%d", rr.Code)
	}
}

// 학습된 속도를 OTP 요청에 넣을 때 자전거는 평균 → 평지 최대속도로 바꿔야 한다(걷기는 그대로 간다).
func TestPlanConvertsLearnedBikeSpeed(t *testing.T) {
	s, pool := testServer(t)
	s.Planner = &route.Planner{OTP: &deadlineOTP{}}
	h := s.Router()
	sub := "sub-bike-" + time.Now().Format("150405.000000")
	_, out := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, "")
	token := out["token"].(string)
	var userID string
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE google_sub = $1`, sub).Scan(&userID); err != nil {
		t.Fatalf("사용자 조회: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM speed_profiles WHERE user_id = $1`, userID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO speed_profiles(user_id, mode, n_trips, speed_mps) VALUES ($1,'bicycle',3,$2),($1,'walk',3,$3)`,
		userID, 3.6, 1.4); err != nil {
		t.Fatalf("프로파일 저장: %v", err)
	}
	rr, out := do(t, h, http.MethodPost, "/routes/plan",
		`{"origin":{"lat":37.5547,"lon":126.9707},"destination":{"lat":37.4979,"lon":127.0276}}`, token)
	if rr.Code != 200 {
		t.Fatalf("code=%d body=%v", rr.Code, out)
	}
	if got := out["bike_speed"].(float64); math.Abs(got-route.BikeOTPSpeed(3.6)) > 1e-9 {
		t.Fatalf("자전거 %v: 학습 평균 3.6 을 변환하지 않고 보냈다", got)
	}
	if got := out["walk_speed"].(float64); math.Abs(got-1.4) > 1e-9 {
		t.Fatalf("걷기 %v: 학습값 그대로여야 한다", got)
	}
}
