// fastexit: 공공데이터포털 「서울교통공사_빠른하차정보」 전체를 받아 otp/data/fast-exit.json 으로 저장한다.
// 서버는 기동할 때 이 파일만 읽는다(요청마다 API 를 부르지 않는다). 자료가 갱신되면 다시 실행한다.
//
//	cd backend && go run ./cmd/fastexit
//
// 키는 .env 의 DATA_GO_KR_KEY(포털에서 이 API 활용신청이 승인돼 있어야 한다). 키·URL 은 출력하지 않는다.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/config"
	"github.com/SIDED00R/seoul-route/backend/internal/fastexit"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	key := cfg.BusArrivalKey
	if key == "" {
		return fmt.Errorf("DATA_GO_KR_KEY 가 없다")
	}
	if d, err := url.QueryUnescape(key); err == nil { // 포털이 주는 키는 URL 인코딩된 형태
		key = d
	}
	rows, err := fastexit.Fetch(&http.Client{Timeout: 60 * time.Second}, fastexit.APIURL, key)
	if err != nil {
		return err
	}
	if err := writeJSON(cfg.FastExitJSON, rows); err != nil {
		return err
	}
	fmt.Printf("빠른하차 %d행 → %s\n", len(rows), cfg.FastExitJSON)
	if cfg.SeoulOpenAPIKey == "" {
		fmt.Println("경고: SEOUL_OPENAPI_KEY 없음 — 에스컬레이터 운행방향은 받지 못했다(계단만 표시된다)")
		return nil
	}
	esc, err := fastexit.FetchEscalators(&http.Client{Timeout: 60 * time.Second},
		fastexit.EscalatorAPIURL, cfg.SeoulOpenAPIKey)
	if err != nil {
		return err
	}
	// 0행을 쓰면 파일이 "null" 이 되고 서버는 오류 없이 빈 방향표로 기동한다 — 에스컬레이터가 무음으로 사라진다.
	// 정적 설비 대장이라 0행은 정상 결과가 아니다.
	if len(esc) == 0 {
		return fmt.Errorf("에스컬레이터 0행 — 기존 %s 를 그대로 둔다", cfg.EscalatorJSON)
	}
	if err := writeJSON(cfg.EscalatorJSON, esc); err != nil {
		return err
	}
	fmt.Printf("에스컬레이터 %d행 → %s\n", len(esc), cfg.EscalatorJSON)
	return nil
}

func writeJSON(path string, rows []json.RawMessage) error {
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
