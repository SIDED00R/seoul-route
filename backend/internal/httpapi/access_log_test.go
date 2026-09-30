package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

// 접근 로그는 타일 요청만 라우트 패턴(좌표 숨김)을 남기고, 나머지는 라우트가 없는 요청까지 요청 경로를 남긴다.
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
	const tripPath = "/trips/22222222-2222-2222-2222-222222222222/traces"
	do(t, h, http.MethodPost, tripPath, `{"samples":[]}`, token)
	do(t, h, http.MethodGet, "/nope/123", "", "")
	logs := buf.String()
	if strings.Contains(logs, "111772") || !strings.Contains(logs, "path=/tiles/{z}/{x}/{y}.png") {
		t.Fatalf("타일 좌표가 로그에 남았다: %s", logs)
	}
	if strings.Contains(logs, "path=/trips/{id}/traces") || !strings.Contains(logs, "path="+tripPath) {
		t.Fatalf("trip 경로는 실제 경로가 남아야 한다: %s", logs)
	}
	if !strings.Contains(logs, "path=/nope/123") {
		t.Fatalf("404 는 요청 경로가 남아야 한다: %s", logs)
	}
}
