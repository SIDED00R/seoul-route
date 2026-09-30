// Package httpapi 는 HTTP 라우팅과 핸들러다. 비즈니스 규칙은 각 핸들러 파일에 둔다.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SIDED00R/seoul-route/backend/internal/auth"
	"github.com/SIDED00R/seoul-route/backend/internal/route"
	"github.com/SIDED00R/seoul-route/backend/internal/shops"
)

type Server struct {
	DB     *pgxpool.Pool
	JWT    *auth.JWT
	Google auth.GoogleVerifier // nil 이면 /auth/google 은 503
	// Allowed 는 이 서버에 로그인할 수 있는 Google 계정 이메일 목록(AUTH_ALLOWED_EMAILS). 비어 있으면 전부 허용.
	Allowed auth.EmailAllowlist
	// GoogleClientID 는 /auth/config 로 앱에 내려주는 웹 클라이언트 ID(Google 이 비밀로 보지 않는 값). Google 이 nil 이면 빈 문자열.
	GoogleClientID string
	OTPURL         string
	HTTP           *http.Client
	Log            *slog.Logger
	Version        string // 빌드한 git 커밋(cmd/api 의 -X main.version). /health 로 나간다. 비면 "dev"

	Planner        *route.Planner
	GBFS           http.Handler // nil 이면 /gbfs/* 은 503
	KakaoKey       string       // 비면 /places/search·/places/reverse·/places/landmark 503
	KakaoBase      string       // 카카오 로컬 API 주소. 비면 KakaoBaseURL(테스트에서만 바꾼다)
	VWorldKey      string       // 비면 /tiles/* 503, /places/reverse 는 VWorld 건물 이름 없이 답한다
	VWorldBase     string       // VWorld API 주소. 비면 VWorldBaseURL(테스트에서만 바꾼다)
	Shops          *shops.Index // nil 이면 /places/search 는 카카오 결과만 쓴다(부분 일치 없음)
	landmarkMu     sync.Mutex
	landmarks      map[string]cachedLandmark
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
		r.With(short).Get("/places/reverse", s.handlePlacesReverse)
		r.With(short).Get("/places/landmark", s.handlePlacesLandmark)
		r.With(short).Get("/tiles/{z}/{x}/{y}.png", s.handleTile)
		r.With(short).Get("/users/me/favorites", s.handleGetFavoritePlaces)
		r.With(short).Post("/users/me/favorites", s.handleCreateFavoritePlace)
		r.With(short).Put("/users/me/favorites/{id}", s.handleUpdateFavoritePlace)
		r.With(short).Delete("/users/me/favorites/{id}", s.handleDeleteFavoritePlace)
		r.With(middleware.Timeout(PlanTimeout)).Post("/routes/plan", s.handlePlan)
		// 최근 경로: 성공한 검색을 기록하고(handlePlan) 홈의 "최근 경로" 탭이 읽는다. recent_routes_handler.go
		r.With(short).Get("/routes/recent", s.handleGetRecentRoutes)
		r.With(short).Delete("/routes/recent", s.handleDeleteRecentRoutes)
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
		if errors.Is(err, pgx.ErrNoRows) || err == nil && deleted {
			writeError(w, http.StatusUnauthorized, "사용자 없음")
			return
		}
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				s.Log.Error("auth user lookup", "err", err)
			}
			writeError(w, http.StatusServiceUnavailable, "사용자 확인 실패")
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
		// 쿼리스트링은 기록하지 않는다. 경로는 라우트 패턴으로 남긴다.
		// 라우트가 없는 요청(404·405)은 패턴이 비어 요청 경로를 남긴다.
		path := chi.RouteContext(r.Context()).RoutePattern()
		if path == "" {
			path = r.URL.Path
		}
		s.Log.Info("http", "method", r.Method, "path", path, "status", ww.Status(),
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
