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

// 접근 로그는 라우트 패턴을 남기고, 라우트가 없는 요청은 요청 경로를 남긴다.
func TestAccessLogUsesRoutePattern(t *testing.T) {
	s, pool := testServer(t)
	var buf bytes.Buffer
	s.Log = slog.New(slog.NewTextHandler(&buf, nil))
	sub := "log-" + strings.ReplaceAll(t.Name(), "/", "-")
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = $1`, sub) })
	h := s.Router()
	_, out := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, "")
	token := out["token"].(string)
	do(t, h, http.MethodGet, "/tiles/17/111772/50786.png", "", token)
	do(t, h, http.MethodGet, "/nope/123", "", "")
	logs := buf.String()
	if strings.Contains(logs, "111772") || !strings.Contains(logs, "path=/tiles/{z}/{x}/{y}.png") {
		t.Fatalf("타일 좌표가 로그에 남았다: %s", logs)
	}
	if !strings.Contains(logs, "path=/nope/123") {
		t.Fatalf("404 는 요청 경로가 남아야 한다: %s", logs)
	}
}
