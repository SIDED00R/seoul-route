// Package httpapi 는 HTTP 라우팅과 핸들러다. 비즈니스 규칙은 각 핸들러 파일에 둔다.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SIDED00R/seoul-route/backend/internal/auth"
	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

type Server struct {
	DB      *pgxpool.Pool
	JWT     *auth.JWT
	Google  auth.GoogleVerifier // nil 이면 /auth/google 은 503
	// GoogleClientID 는 /auth/config 로 앱에 내려주는 웹 클라이언트 ID(Google 이 비밀로 보지 않는 값). Google 이 nil 이면 빈 문자열.
	GoogleClientID string
	OTPURL  string
	HTTP    *http.Client
	Log     *slog.Logger
	Planner *route.Planner
	GBFS    http.Handler // nil 이면 /gbfs/* 은 503
	KakaoKey  string // 비면 /places/search 503
	VWorldKey string // 비면 /tiles/* 503
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(s.logRequests)
	// chi Timeout 은 부모 컨텍스트 데드라인을 늘릴 수 없으므로 전역에 걸지 않고 라우트별로 건다.
	short := middleware.Timeout(DefaultTimeout)
	r.With(short).Get("/health", s.handleHealth)
	r.With(short).Get("/auth/config", s.handleAuthConfig)
	r.With(short).Post("/auth/google", s.handleAuthGoogle)
	r.With(short).Get("/gbfs/*", func(w http.ResponseWriter, req *http.Request) {
		if s.GBFS == nil {
			writeError(w, http.StatusServiceUnavailable, "GBFS 미설정")
			return
		}
		s.GBFS.ServeHTTP(w, req)
	})
	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.With(short).Get("/users/me", s.handleGetMe)
		r.With(short).Delete("/users/me", s.handleDeleteMe)
		r.With(short).Get("/users/me/speed", s.handleGetSpeed)
		// 안내 궤적: trip 발급 → 샘플 배치 업로드(멱등) → 종료(속도 학습). trips_handler.go
		r.With(short).Post("/trips", s.handleStartTrip)
		r.With(short).Post("/trips/{id}/traces", s.handleUploadTraces)
		r.With(short).Post("/trips/{id}/end", s.handleEndTrip)
		r.With(short).Get("/places/search", s.handlePlacesSearch)
		r.With(short).Get("/tiles/{z}/{x}/{y}.png", s.handleTile)
		// via 대중교통 탐색이 OTP 에서 15~37초 걸린다(실측) → 이 경로만 상한이 길다.
		r.With(middleware.Timeout(PlanTimeout)).Post("/routes/plan", s.handlePlan)
	})
	return r
}

const (
	DefaultTimeout = 30 * time.Second
	// PlanTimeout: /routes/plan 의 요청 상한. OTP 클라이언트 타임아웃도 이 값 이상이어야 한다(main.go).
	PlanTimeout = 60 * time.Second
)

type ctxKey int

const ctxUserID ctxKey = 1

// requireAuth 는 Bearer JWT 를 검증하고, 탈퇴한 사용자는 거부한다.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "Bearer 토큰 필요")
			return
		}
		userID, err := s.JWT.Verify(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "토큰 무효")
			return
		}
		var deleted bool
		err = s.DB.QueryRow(r.Context(),
			`SELECT deleted_at IS NOT NULL FROM users WHERE id = $1`, userID).Scan(&deleted)
		if err != nil || deleted {
			writeError(w, http.StatusUnauthorized, "사용자 없음")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUserID, userID)))
	})
}

func userIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxUserID).(string)
	return id
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		// 쿼리스트링은 기록하지 않는다(키·토큰이 섞일 수 있다).
		s.Log.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(),
			"ms", time.Since(start).Milliseconds(), "req", middleware.GetReqID(r.Context()))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
