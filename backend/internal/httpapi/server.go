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
)

type Server struct {
	DB     *pgxpool.Pool
	JWT    *auth.JWT
	Google auth.GoogleVerifier // nil 이면 /auth/google 은 503
	OTPURL string
	HTTP   *http.Client
	Log    *slog.Logger
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, middleware.Timeout(30*time.Second))
	r.Use(s.logRequests)
	r.Get("/health", s.handleHealth)
	r.Post("/auth/google", s.handleAuthGoogle)
	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/users/me", s.handleGetMe)
		r.Delete("/users/me", s.handleDeleteMe)
	})
	return r
}

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
