package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/auth"
)

// AUTH_ALLOWED_EMAILS 가 있으면 목록 밖 계정·미확인 이메일은 403 이고 사용자 행을 만들지 않는다. 목록 안 계정만 JWT 를 받는다.
func TestAuthGoogleAllowlist(t *testing.T) {
	s, pool := testServer(t)
	stamp := time.Now().Format("150405.000000")
	okSub, badSub, unvSub := "allow-"+stamp, "deny-"+stamp, "unv-"+stamp
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = ANY($1)`, []string{okSub, badSub, unvSub})
	})
	s.Allowed = auth.ParseEmailAllowlist(" " + okSub + "@EXAMPLE.com ," + unvSub + "@example.com")
	h := s.Router()

	rr, out := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+badSub+`"}`, "")
	if rr.Code != 403 || out["token"] != nil {
		t.Fatalf("목록 밖 계정 code=%d body=%v", rr.Code, out)
	}
	if rr, _ := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"unverified:`+unvSub+`"}`, ""); rr.Code != 403 {
		t.Fatalf("미확인 이메일 code=%d", rr.Code)
	}
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM users WHERE google_sub = ANY($1)`,
		[]string{badSub, unvSub}).Scan(&n)
	if n != 0 {
		t.Fatalf("거부된 계정의 사용자 행이 생겼다: %d", n)
	}
	rr, out = do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+okSub+`"}`, "")
	if rr.Code != 200 || out["token"] == nil {
		t.Fatalf("목록 안 계정 code=%d body=%v", rr.Code, out)
	}
	if rr, _ := do(t, h, http.MethodGet, "/users/me", "", out["token"].(string)); rr.Code != 200 {
		t.Fatalf("허용 계정 토큰이 보호 경로를 못 지났다: code=%d", rr.Code)
	}
}
