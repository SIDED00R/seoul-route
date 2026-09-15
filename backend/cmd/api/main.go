// api: seoul-route 백엔드 서버. 설정은 .env/환경변수(internal/config).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/auth"
	"github.com/SIDED00R/seoul-route/backend/internal/config"
	"github.com/SIDED00R/seoul-route/backend/internal/crossing"
	"github.com/SIDED00R/seoul-route/backend/internal/db"
	"github.com/SIDED00R/seoul-route/backend/internal/gbfs"
	"github.com/SIDED00R/seoul-route/backend/internal/headway"
	"github.com/SIDED00R/seoul-route/backend/internal/httpapi"
	"github.com/SIDED00R/seoul-route/backend/internal/otp"
	"github.com/SIDED00R/seoul-route/backend/internal/realtime"
	"github.com/SIDED00R/seoul-route/backend/internal/route"
	"github.com/SIDED00R/seoul-route/backend/internal/speed"
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
	// 원본 궤적 30일 보관(speed.TraceRetention). 기동 직후 한 번, 이후 1시간마다.
	go func() {
		for {
			if n, err := speed.PurgeOldTraces(ctx, pool, time.Now()); err != nil {
				log.Warn("trace purge", "err", err)
			} else if n > 0 {
				log.Info("trace purge", "deleted", n)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Hour):
			}
		}
	}()

	var google auth.GoogleVerifier
	googleClientID := ""
	if g, err := auth.NewGoogleVerifier(cfg.GoogleOAuthClientID); err == nil {
		google = g
		googleClientID = cfg.GoogleOAuthClientID
	} else {
		log.Warn("google login disabled", "reason", err.Error())
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	otpClient := &otp.Client{URL: cfg.OTPURL, HTTP: &http.Client{Timeout: httpapi.PlanTimeout}}
	planner := &route.Planner{OTP: otpClient, CrossingSec: crossing.ExpectedWaitSec}
	// 신호 횡단보도 대기(otp/extract_crossings.py 산출). 없으면 도보 시간에 대기가 빠진 채 계산된다.
	if ix, err := crossing.Load(cfg.CrossingsCSV); err != nil {
		log.Warn("crossing wait disabled", "path", cfg.CrossingsCSV, "err", err)
	} else {
		planner.Crossings = ix
		log.Info("crossings loaded", "n", ix.Len(), "wait_sec", crossing.ExpectedWaitSec)
	}
	// 버스 배차간격(앱 "배차 약 N분")은 생성 GTFS 의 frequencies.txt 에서. 없으면 표시만 빠진다.
	if hw, err := headway.Load(cfg.GTFSZip); err != nil {
		log.Warn("headways disabled", "path", cfg.GTFSZip, "err", err)
	} else {
		planner.Headways = hw
		log.Info("headways loaded", "routes", len(hw))
	}
	// 첫 탑승 실시간 보정. 키가 있는 수단만 켠다. 외부 API 는 응답이 느릴 수 있어 짧은 타임아웃.
	rt := &realtime.Corrector{Log: log}
	rtHTTP := &http.Client{Timeout: 5 * time.Second}
	if cfg.BusArrivalKey != "" {
		key := cfg.BusArrivalKey
		if d, err := url.QueryUnescape(key); err == nil { // 포털이 주는 키는 URL 인코딩된 형태(gtfsgen 과 같은 처리)
			key = d
		}
		rt.Bus = &realtime.BusClient{Key: key, HTTP: rtHTTP}
	} else {
		log.Warn("bus realtime disabled", "reason", "DATA_GO_KR_KEY 없음")
	}
	if cfg.SubwayRealtimeKey != "" {
		rt.Subway = &realtime.SubwayClient{Key: cfg.SubwayRealtimeKey, HTTP: rtHTTP}
	} else {
		log.Warn("subway realtime disabled", "reason", "SEOUL_SUBWAY_REALTIME_KEY 없음")
	}
	if rt.Bus != nil || rt.Subway != nil {
		planner.Realtime = rt
	}
	// 부모역 목록은 OTP 에서 한 번 받는다. Compose 에서는 OTP 가 그래프 로드에 1~2분 걸려 api 보다 늦게 뜨므로
	// 될 때까지 30초마다 재시도하고, 그동안은 앵커링 없이(좌표로) 동작한다.
	go func() {
		for {
			sts, err := otpClient.Stations(ctx)
			if err == nil {
				planner.SetStations(sts)
				log.Info("stations loaded", "n", len(sts))
				return
			}
			log.Warn("stations load failed, retrying in 30s", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
		}
	}()
	srv := &httpapi.Server{
		DB: pool, JWT: auth.NewJWT(cfg.JWTSecret), Google: google, GoogleClientID: googleClientID,
		OTPURL: cfg.OTPURL, HTTP: httpClient, Log: log,
		Planner: planner, KakaoKey: cfg.KakaoRESTKey, VWorldKey: cfg.VWorldKey,
	}
	if cfg.KakaoRESTKey == "" {
		log.Warn("places search disabled", "reason", "KAKAO_REST_API_KEY 없음")
	}
	if cfg.VWorldKey == "" {
		log.Warn("map tiles disabled", "reason", "VWORLD_API_KEY 없음")
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
