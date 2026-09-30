package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SIDED00R/seoul-route/backend/internal/auth"
)

// 닿지 않는 DB 에서 유효한 토큰은 401 이 아니라 503 을 받고, 원인이 로그에 남는다.
func TestRequireAuthDBErrorIs503(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://seoul:seoul@127.0.0.1:1/none?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var buf bytes.Buffer
	jwt := auth.NewJWT(strings.Repeat("t", 32))
	s := &Server{DB: pool, JWT: jwt, Log: slog.New(slog.NewTextHandler(&buf, nil))}
	token, err := jwt.Issue("11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	rr, out := do(t, s.Router(), http.MethodGet, "/users/me", "", token)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d body=%v", rr.Code, out)
	}
	if !strings.Contains(buf.String(), "auth user lookup") {
		t.Fatalf("원인 로그가 없다: %s", buf.String())
	}
}

func TestRequireAuthUnknownUserIs401(t *testing.T) {
	s, _ := testServer(t)
	token, err := s.JWT.Issue("11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if rr, out := do(t, s.Router(), http.MethodGet, "/users/me", "", token); rr.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%v", rr.Code, out)
	}
}
