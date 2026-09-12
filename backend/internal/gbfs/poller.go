// Package gbfs 는 서울 열린데이터광장 따릉이 bikeList 를 GBFS 2.3 피드로 바꿔 OTP 에 제공한다.
// 폴링 결과는 메모리 스냅샷 하나로 유지하고, 상류가 실패하면 마지막 정상 스냅샷을 stale 표시와 함께 계속 낸다.
package gbfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 상수 출처
//   - PageSize 1000: 열린데이터광장 1콜 최대 행수(문서·실측 2026-09-12).
//   - PollInterval 60s: GBFS station_status 권고 ttl 과 같다. 하루 4,320콜 = 1,440회 × 3페이지.
//   - MaxAge 10m: 이보다 오래된 스냅샷은 stale 로 표시하고 is_renting=false 로 내보내 유령 대여를 막는다.
const (
	PageSize     = 1000
	PollInterval = 60 * time.Second
	MaxAge       = 10 * time.Minute
	SystemID     = "ttareungi"
	// 열린데이터광장은 443 미개방(실측 2026-09-12) → http. 키는 URL 경로에 들어가므로 URL 을 절대 기록하지 않는다.
	DefaultBaseURL = "http://openapi.seoul.go.kr:8088"
)

type Station struct {
	ID       string
	Name     string
	Lat, Lon float64
	Capacity int
	Bikes    int
}

type Snapshot struct {
	Stations  []Station
	FetchedAt time.Time
}

type Poller struct {
	BaseURL string
	key     string
	http    *http.Client
	log     *slog.Logger
	now     func() time.Time
	mu      sync.RWMutex
	snap    *Snapshot
	fails   int
}

func NewPoller(key string, log *slog.Logger) *Poller {
	return &Poller{BaseURL: DefaultBaseURL, key: key, http: &http.Client{Timeout: 30 * time.Second}, log: log,
		now: time.Now}
}

// Run 은 즉시 1회 수집한 뒤 PollInterval 마다 반복한다. ctx 취소로 끝난다.
func (p *Poller) Run(ctx context.Context) {
	p.pollOnce(ctx)
	t := time.NewTicker(PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.pollOnce(ctx)
		}
	}
}

func (p *Poller) pollOnce(ctx context.Context) {
	stations, err := p.fetchAll(ctx)
	if err != nil {
		p.mu.Lock()
		p.fails++
		fails := p.fails
		p.mu.Unlock()
		p.log.Warn("gbfs poll failed", "err", err.Error(), "consecutive", fails)
		return
	}
	p.mu.Lock()
	p.snap = &Snapshot{Stations: stations, FetchedAt: p.now()}
	p.fails = 0
	p.mu.Unlock()
	p.log.Info("gbfs poll ok", "stations", len(stations))
}

// Current 는 마지막 정상 스냅샷과 stale 여부(MaxAge 초과 또는 스냅샷 없음)를 돌려준다.
func (p *Poller) Current() (*Snapshot, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.snap == nil {
		return nil, true
	}
	return p.snap, p.now().Sub(p.snap.FetchedAt) > MaxAge
}

// fetchAll 은 1000행씩 짧은 페이지가 나올 때까지 넘긴다. list_total_count 는 요청 범위 건수라 쓰지 않는다(실측).
func (p *Poller) fetchAll(ctx context.Context) ([]Station, error) {
	var all []Station
	seen := map[string]bool{}
	for page := 1; page <= 10; page++ {
		rows, err := p.fetchPage(ctx, (page-1)*PageSize+1, page*PageSize)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		for _, r := range rows {
			s, err := r.station()
			if err != nil || seen[s.ID] {
				continue
			}
			seen[s.ID] = true
			all = append(all, s)
		}
		if len(rows) < PageSize {
			break
		}
	}
	if len(all) == 0 {
		return nil, errors.New("대여소 0건")
	}
	return all, nil
}

type row struct {
	StationID string `json:"stationId"`
	Name      string `json:"stationName"`
	Lat       string `json:"stationLatitude"`
	Lon       string `json:"stationLongitude"`
	Rack      string `json:"rackTotCnt"`
	Bikes     string `json:"parkingBikeTotCnt"`
}

func (r row) station() (Station, error) {
	lat, e1 := strconv.ParseFloat(strings.TrimSpace(r.Lat), 64)
	lon, e2 := strconv.ParseFloat(strings.TrimSpace(r.Lon), 64)
	rack, e3 := strconv.Atoi(strings.TrimSpace(r.Rack))
	bikes, e4 := strconv.Atoi(strings.TrimSpace(r.Bikes))
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || r.StationID == "" || lat == 0 || lon == 0 {
		return Station{}, errors.New("행 파싱 실패")
	}
	return Station{ID: r.StationID, Name: r.Name, Lat: lat, Lon: lon, Capacity: rack, Bikes: bikes}, nil
}

func (p *Poller) fetchPage(ctx context.Context, start, end int) ([]row, error) {
	url := fmt.Sprintf("%s/%s/json/bikeList/%d/%d/", p.BaseURL, p.key, start, end)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.New("요청 생성 실패")
	}
	resp, err := p.http.Do(req)
	if err != nil {
		// *url.Error 는 키가 든 URL 을 포함하므로 원인만 남긴다.
		var ue interface{ Unwrap() error }
		if errors.As(err, &ue) {
			err = ue.Unwrap()
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var body struct {
		Status struct {
			Result struct {
				Code    string `json:"CODE"`
				Message string `json:"MESSAGE"`
			} `json:"RESULT"`
			Row []row `json:"row"`
		} `json:"rentBikeStatus"`
		Result struct {
			Code    string `json:"CODE"`
			Message string `json:"MESSAGE"`
		} `json:"RESULT"`
		// 범위 밖 요청(무자료)은 래퍼 없이 평면 {"CODE":"INFO-200","MESSAGE":"해당하는 데이터가 없습니다."} 로 온다(실측 2026-09-12).
		Code    string `json:"CODE"`
		Message string `json:"MESSAGE"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, errors.New("JSON 아님")
	}
	if body.Code == "INFO-200" {
		return nil, nil // 빈 페이지. 총수가 1,000 의 배수면 마지막 꽉 찬 페이지 다음에 이게 온다.
	}
	if body.Code != "" && body.Code != "INFO-000" {
		return nil, fmt.Errorf("API %s %s", body.Code, body.Message)
	}
	if body.Result.Code != "" && body.Result.Code != "INFO-000" {
		return nil, fmt.Errorf("API %s %s", body.Result.Code, body.Result.Message)
	}
	if body.Status.Result.Code != "INFO-000" {
		return nil, fmt.Errorf("API %s %s", body.Status.Result.Code, body.Status.Result.Message)
	}
	return body.Status.Row, nil
}
