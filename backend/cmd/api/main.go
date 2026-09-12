// api: seoul-route 백엔드 서버. 설정은 .env/환경변수(internal/config).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/auth"
	"github.com/SIDED00R/seoul-route/backend/internal/config"
	"github.com/SIDED00R/seoul-route/backend/internal/db"
	"github.com/SIDED00R/seoul-route/backend/internal/gbfs"
	"github.com/SIDED00R/seoul-route/backend/internal/httpapi"
	"github.com/SIDED00R/seoul-route/backend/internal/otp"
	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	applied, err := db.Migrate(ctx, pool)
	if err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}
	log.Info("migrate", "applied", applied)

	var google auth.GoogleVerifier
	if g, err := auth.NewGoogleVerifier(cfg.GoogleOAuthClientID); err == nil {
		google = g
	} else {
		log.Warn("google login disabled", "reason", err.Error())
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	srv := &httpapi.Server{
		DB: pool, JWT: auth.NewJWT(cfg.JWTSecret), Google: google, OTPURL: cfg.OTPURL, HTTP: httpClient, Log: log,
		Planner: &route.Planner{OTP: &otp.Client{URL: cfg.OTPURL, HTTP: &http.Client{Timeout: httpapi.PlanTimeout}}},
	}
	if cfg.SeoulOpenAPIKey != "" {
		poller := gbfs.NewPoller(cfg.SeoulOpenAPIKey, log)
		go poller.Run(ctx)
		srv.GBFS = &gbfs.Handler{Poller: poller, BaseURL: cfg.PublicURL + "/gbfs"}
	} else {
		log.Warn("gbfs disabled", "reason", "SEOUL_OPENAPI_KEY 없음")
	}
	// ReadTimeout 은 본문까지 포함한 요청 전체 읽기 상한(느린 본문으로 goroutine 이 무기한 묶이는 것을 막는다).
	// IdleTimeout 을 안 주면 keep-alive 유휴 연결도 ReadTimeout 에 닫힌다.
	hs := &http.Server{
		Addr: ":" + cfg.Port, Handler: srv.Router(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hs.Shutdown(shutdownCtx)
	}()
	log.Info("listen", "addr", hs.Addr, "otp", cfg.OTPURL)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
