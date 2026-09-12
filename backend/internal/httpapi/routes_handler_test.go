package httpapi

import (
	"context"
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
