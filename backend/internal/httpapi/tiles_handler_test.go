package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func tileRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Get("/tiles/{z}/{x}/{y}.png", s.handleTile)
	return r
}

func getTile(h http.Handler, ctx context.Context) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tiles/15/27944/12693.png", nil).WithContext(ctx))
	return rec
}

// 상류가 한 번 실패(5xx)해도 다시 보내 타일을 준다. VWorld 는 {z}/{y}/{x} 순서다.
func TestTileRetriesOnce(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/req/wmts/1.0.0/KEY/Base/15/12693/27944.png" {
			t.Errorf("상류 경로: %s", r.URL.Path)
		}
		if calls == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte("PNG"))
	}))
	defer up.Close()
	var logs bytes.Buffer
	s := &Server{VWorldKey: "KEY", VWorldBase: up.URL, HTTP: up.Client(), Log: slog.New(slog.NewTextHandler(&logs, nil))}
	rec := getTile(tileRouter(s), context.Background())
	if rec.Code != http.StatusOK || rec.Body.String() != "PNG" || calls != 2 {
		t.Fatalf("code=%d body=%q calls=%d", rec.Code, rec.Body.String(), calls)
	}
	if logs.Len() != 0 {
		t.Errorf("성공했는데 경고가 남았다: %s", logs.String())
	}
}

func TestTileFailureKinds(t *testing.T) {
	cases := []struct {
		name      string
		upstream  http.HandlerFunc
		timeout   time.Duration
		wantCalls int
		wantLog   string
	}{
		{"5xx 가 이어지면 두 번 보내고 상태코드를 남긴다",
			func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }, 0, 2,
			`err="status 503" attempts=2`},
		{"4xx 는 다시 보내지 않는다",
			func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }, 0, 1,
			`err="status 404" attempts=1`},
		{"시간 초과",
			func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) }, 50 * time.Millisecond, 2,
			"err=timeout attempts=2"},
	}
	for _, c := range cases {
		var calls int32
		up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			c.upstream(w, r)
		}))
		var logs bytes.Buffer
		hc := up.Client()
		hc.Timeout = c.timeout
		s := &Server{VWorldKey: "SECRETKEY", VWorldBase: up.URL, HTTP: hc, Log: slog.New(slog.NewTextHandler(&logs, nil))}
		rec := getTile(tileRouter(s), context.Background())
		up.Close()
		if got := atomic.LoadInt32(&calls); rec.Code != http.StatusBadGateway || got != int32(c.wantCalls) {
			t.Errorf("%s: code=%d calls=%d", c.name, rec.Code, got)
		}
		if !strings.Contains(logs.String(), c.wantLog) || strings.Contains(logs.String(), "SECRETKEY") {
			t.Errorf("%s: 로그=%s", c.name, logs.String())
		}
	}
}

// 연결이 안 되면 transport 로 남기고, 키가 든 URL 은 로그에 없다.
func TestTileTransportError(t *testing.T) {
	var logs bytes.Buffer
	s := &Server{VWorldKey: "SECRETKEY", VWorldBase: "http://127.0.0.1:9", HTTP: &http.Client{Timeout: 2 * time.Second},
		Log: slog.New(slog.NewTextHandler(&logs, nil))}
	rec := getTile(tileRouter(s), context.Background())
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code=%d", rec.Code)
	}
	if !strings.Contains(logs.String(), "err=transport") || strings.Contains(logs.String(), "SECRETKEY") {
		t.Errorf("로그=%s", logs.String())
	}
}

// 앱이 요청을 거두면(지도를 넘김) 상류에 다시 닿지 않고 경고도 남기지 않는다.
func TestTileCanceledByClient(t *testing.T) {
	var calls int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(200 * time.Millisecond)
	}))
	var logs bytes.Buffer
	s := &Server{VWorldKey: "KEY", VWorldBase: up.URL, HTTP: up.Client(), Log: slog.New(slog.NewTextHandler(&logs, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	getTile(tileRouter(s), ctx)
	up.Close() // 남은 핸들러가 끝나야 카운터를 읽을 수 있다
	if got := atomic.LoadInt32(&calls); got != 1 || logs.Len() != 0 {
		t.Errorf("calls=%d 로그=%s", got, logs.String())
	}
}

// 요청 전체 데드라인 안에서 재시도가 상류에 닿는다. 시도마다 예산을 따로 주지 않으면 첫 시도가 그 데드라인을 다 쓰고
// 끝나 두 번째가 상류에 닿지 못한다 — 운영 배선(라우트 30초 + 클라이언트 30초)이 정확히 그 모양이다.
func TestTileRetryHasOwnBudget(t *testing.T) {
	defer func(d time.Duration) { tileAttemptTimeout = d }(tileAttemptTimeout)
	tileAttemptTimeout = 40 * time.Millisecond
	var calls int32
	// 상류 지연을 라우트 데드라인보다 길게 둔다 — 그래야 시도별 예산이 없을 때 첫 시도가 데드라인을 다 쓰고
	// 끝나는 실제 증상(상류 1회 접촉·502)을 태운다.
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(600 * time.Millisecond)
	}))
	var logs bytes.Buffer
	s := &Server{VWorldKey: "KEY", VWorldBase: up.URL, HTTP: up.Client(), Log: slog.New(slog.NewTextHandler(&logs, nil))}
	router := chi.NewRouter()
	router.Use(middleware.Timeout(500 * time.Millisecond))
	router.Get("/tiles/{z}/{x}/{y}.png", s.handleTile)
	rec := getTile(router, context.Background())
	up.Close()
	if got := atomic.LoadInt32(&calls); rec.Code != http.StatusBadGateway || got != 2 {
		t.Fatalf("code=%d calls=%d", rec.Code, got)
	}
	if !strings.Contains(logs.String(), "err=timeout attempts=2") {
		t.Errorf("로그=%s", logs.String())
	}
}

// 성공한 시도의 컨텍스트는 본문을 다 읽을 때까지 살아 있어야 한다. 곧바로 거두면 타일이 중간에 잘린다.
func TestTileBodyNotTruncated(t *testing.T) {
	png := bytes.Repeat([]byte("P"), 512<<10)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(png) }))
	defer up.Close()
	s := &Server{VWorldKey: "KEY", VWorldBase: up.URL, HTTP: up.Client(), Log: slog.New(slog.DiscardHandler)}
	rec := getTile(tileRouter(s), context.Background())
	if rec.Code != http.StatusOK || rec.Body.Len() != len(png) {
		t.Fatalf("code=%d 받은 바이트=%d (보낸 %d)", rec.Code, rec.Body.Len(), len(png))
	}
}

func TestTileBadRequests(t *testing.T) {
	s := &Server{Log: slog.New(slog.DiscardHandler)}
	if rec := getTile(tileRouter(s), context.Background()); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("키 없음: %d", rec.Code)
	}
	s.VWorldKey = "KEY"
	rec := httptest.NewRecorder()
	tileRouter(s).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tiles/3/1/1.png", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("줌 범위 밖: %d", rec.Code)
	}
}
