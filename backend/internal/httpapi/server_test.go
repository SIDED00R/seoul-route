package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SIDED00R/seoul-route/backend/internal/auth"
	"github.com/SIDED00R/seoul-route/backend/internal/db"
)

// 실제 PostgreSQL 이 필요하다. TEST_DATABASE_URL 이 없으면 건너뛴다(배선을 타는 테스트만 인정하는 규칙).
func testServer(t *testing.T) (*Server, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 없음")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	otp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"__typename":"QueryType"}}`))
	}))
	t.Cleanup(otp.Close)
	return &Server{
		DB: pool, JWT: auth.NewJWT(strings.Repeat("t", 32)), Google: fakeGoogle{},
		OTPURL: otp.URL, HTTP: otp.Client(), Log: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}, pool
}

// fakeGoogle: "good:<sub>" 형식만 통과시킨다(이메일 <sub>@example.com, 확인됨). "unverified:<sub>" 는 미확인 이메일.
type fakeGoogle struct{}

func (fakeGoogle) Verify(_ context.Context, tok string) (auth.Identity, error) {
	if sub, ok := strings.CutPrefix(tok, "good:"); ok {
		return auth.Identity{Sub: sub, Email: sub + "@example.com", EmailVerified: true}, nil
	}
	if sub, ok := strings.CutPrefix(tok, "unverified:"); ok {
		return auth.Identity{Sub: sub, Email: sub + "@example.com", EmailVerified: false}, nil
	}
	return auth.Identity{}, errors.New("bad token")
}

func do(t *testing.T, h http.Handler, method, path, body, bearer string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var out map[string]any
	if rr.Body.Len() > 0 {
		json.Unmarshal(rr.Body.Bytes(), &out)
	}
	return rr, out
}

func TestHealth(t *testing.T) {
	s, _ := testServer(t)
	rr, out := do(t, s.Router(), http.MethodGet, "/health", "", "")
	if rr.Code != 200 || out["db"] != "ok" || out["otp"] != "ok" {
		t.Fatalf("code=%d body=%v", rr.Code, out)
	}
	s.OTPURL = "http://127.0.0.1:1"
	s.HTTP = &http.Client{Timeout: time.Second}
	rr, out = do(t, s.Router(), http.MethodGet, "/health", "", "")
	if rr.Code != 503 || out["db"] != "ok" || !strings.HasPrefix(out["otp"].(string), "fail") {
		t.Fatalf("OTP 죽었을 때 503 이어야 한다: code=%d body=%v", rr.Code, out)
	}
}

// /auth/config 는 무인증으로 웹 클라이언트 ID 를 준다. 미설정이면 빈 문자열.
func TestAuthConfig(t *testing.T) {
	s, _ := testServer(t)
	rr, out := do(t, s.Router(), http.MethodGet, "/auth/config", "", "")
	if rr.Code != 200 || out["google_client_id"] != "" {
		t.Fatalf("미설정 code=%d body=%v", rr.Code, out)
	}
	s.GoogleClientID = "123.apps.googleusercontent.com"
	_, out = do(t, s.Router(), http.MethodGet, "/auth/config", "", "")
	if out["google_client_id"] != "123.apps.googleusercontent.com" {
		t.Fatalf("body=%v", out)
	}
}

func TestAuthAndUserLifecycle(t *testing.T) {
	s, pool := testServer(t)
	h := s.Router()
	sub := "sub-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = $1`, sub) })

	// 잘못된 Google 토큰 → 401, 본문 없음 → 400
	if rr, _ := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"bad"}`, ""); rr.Code != 401 {
		t.Fatalf("bad token code=%d", rr.Code)
	}
	if rr, _ := do(t, h, http.MethodPost, "/auth/google", `{}`, ""); rr.Code != 400 {
		t.Fatalf("empty body code=%d", rr.Code)
	}
	// 로그인 → JWT
	rr, out := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, "")
	if rr.Code != 200 || out["token"] == nil {
		t.Fatalf("login code=%d body=%v", rr.Code, out)
	}
	token := out["token"].(string)
	userID := out["user_id"].(string)
	// 같은 sub 재로그인은 같은 사용자
	if _, out2 := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, ""); out2["user_id"] != userID {
		t.Fatalf("재로그인 user_id 불일치: %v vs %v", out2["user_id"], userID)
	}
	// 보호 경로: 토큰 없음 401, 있으면 200
	if rr, _ := do(t, h, http.MethodGet, "/users/me", "", ""); rr.Code != 401 {
		t.Fatalf("no bearer code=%d", rr.Code)
	}
	if rr, out := do(t, h, http.MethodGet, "/users/me", "", token); rr.Code != 200 || out["user_id"] != userID {
		t.Fatalf("me code=%d body=%v", rr.Code, out)
	}
	// 탈퇴 → 204, 이후 같은 토큰 401, 같은 계정 재로그인 401
	if rr, _ := do(t, h, http.MethodDelete, "/users/me", "", token); rr.Code != 204 {
		t.Fatalf("delete code=%d", rr.Code)
	}
	if rr, _ := do(t, h, http.MethodGet, "/users/me", "", token); rr.Code != 401 {
		t.Fatalf("탈퇴 후 토큰이 통과했다: code=%d", rr.Code)
	}
	if rr, _ := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, ""); rr.Code != 401 {
		t.Fatalf("탈퇴 계정 재로그인 code=%d", rr.Code)
	}
	var deleted bool
	pool.QueryRow(context.Background(), `SELECT deleted_at IS NOT NULL FROM users WHERE id = $1`, userID).Scan(&deleted)
	if !deleted {
		t.Fatal("deleted_at 이 찍히지 않았다")
	}
}
