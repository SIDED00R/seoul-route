package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SubwayClient 는 서울 열린데이터광장 실시간 지하철 도착(realtimeStationArrival) 클라이언트. 역 이름당 1콜.
type SubwayClient struct {
	Key  string
	HTTP *http.Client
	Base string // 기본 http://swopenapi.seoul.go.kr

	mu    sync.Mutex
	cache map[string]subwayEntry // 역 기준명 → 열차 목록
}

type subwayEntry struct {
	at     time.Time
	trains []SubwayTrain
}

// SubwayTrain 은 역에 접근 중인 열차 하나.
type SubwayTrain struct {
	SubwayID string // "1004" 등(열린데이터광장 노선 코드)
	NextStop string // trainLineNm "불암산행 - 회현방면" 의 "회현"
	ETASec   int    // barvlDt 에서 캐시 경과시간을 뺀 값. 음수면 이미 지나감
	Known    bool   // 조회 시점에 ETA 를 믿을 수 있었는지(barvlDt>0 또는 도착·진입 arvlCd 0/1). 경과 차감과 무관
}

// Trains 는 역(기준명, 예 "서울")에 접근 중인 열차 목록을 돌려준다. ETASec 은 지금 기준(캐시 경과시간 차감, 사본).
func (c *SubwayClient) Trains(ctx context.Context, station string) ([]SubwayTrain, error) {
	c.mu.Lock()
	e, ok := c.cache[station]
	c.mu.Unlock()
	if !ok || time.Since(e.at) > CacheTTL {
		trains, err := c.fetch(ctx, station)
		if err != nil {
			return nil, err
		}
		e = subwayEntry{at: time.Now(), trains: trains}
		c.mu.Lock()
		if c.cache == nil {
			c.cache = map[string]subwayEntry{}
		}
		c.cache[station] = e
		c.mu.Unlock()
	}
	age := int(time.Since(e.at).Seconds())
	out := make([]SubwayTrain, len(e.trains))
	for i, t := range e.trains {
		t.ETASec -= age
		out[i] = t
	}
	return out, nil
}

func (c *SubwayClient) fetch(ctx context.Context, station string) ([]SubwayTrain, error) {
	base := c.Base
	if base == "" {
		base = "http://swopenapi.seoul.go.kr"
	}
	u := fmt.Sprintf("%s/api/subway/%s/json/realtimeStationArrival/0/30/%s", base, c.Key, url.PathEscape(station))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("subway arrival: 요청 실패") // URL 에 키가 있어 원문은 남기지 않는다
	}
	defer resp.Body.Close()
	var out struct {
		ErrorMessage struct {
			Status  int    `json:"status"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errorMessage"`
		List []struct {
			SubwayID    string `json:"subwayId"`
			TrainLineNm string `json:"trainLineNm"`
			BarvlDt     string `json:"barvlDt"`
			ArvlCd      string `json:"arvlCd"`
		} `json:"realtimeArrivalList"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("subway arrival: 응답 파싱 실패 (HTTP %d)", resp.StatusCode)
	}
	if len(out.List) == 0 && out.ErrorMessage.Code != "" && out.ErrorMessage.Code != "INFO-000" {
		return nil, fmt.Errorf("subway arrival: %s (%s)", out.ErrorMessage.Message, out.ErrorMessage.Code)
	}
	trains := make([]SubwayTrain, 0, len(out.List))
	for _, r := range out.List {
		eta, _ := strconv.Atoi(strings.TrimSpace(r.BarvlDt))
		t := SubwayTrain{SubwayID: r.SubwayID, NextStop: nextStopOf(r.TrainLineNm), ETASec: eta,
			Known: eta > 0 || r.ArvlCd == "0" || r.ArvlCd == "1"}
		trains = append(trains, t)
	}
	return trains, nil
}

// nextStopOf 는 "불암산행 - 회현방면" → "회현". "(급행) (막차)" 같은 꼬리표는 뗀다.
func nextStopOf(trainLineNm string) string {
	_, after, ok := strings.Cut(trainLineNm, " - ")
	if !ok {
		return ""
	}
	after = strings.TrimSpace(after)
	if i := strings.Index(after, "방면"); i >= 0 {
		after = after[:i]
	}
	return strings.TrimSpace(after)
}

// subwayIDs: OTP 노선 짧은 이름(파일럿 GTFS "서울4호선", "경의중앙선" 등)에 들어 있는 낱말 → 열린데이터광장 subwayId.
var subwayIDs = []struct{ key, id string }{
	{"1호선", "1001"}, {"2호선", "1002"}, {"3호선", "1003"}, {"4호선", "1004"}, {"5호선", "1005"},
	{"6호선", "1006"}, {"7호선", "1007"}, {"8호선", "1008"}, {"9호선", "1009"},
	{"경의중앙", "1063"}, {"공항철도", "1065"}, {"경춘", "1067"}, {"수인분당", "1075"}, {"신분당", "1077"},
	{"우이신설", "1092"}, {"서해", "1093"}, {"경강", "1081"}, {"GTX-A", "1032"},
}

// SubwayIDFor 는 노선 짧은 이름으로 subwayId 를 찾는다. 모르면 "".
func SubwayIDFor(routeShortName string) string {
	for _, e := range subwayIDs {
		if strings.Contains(routeShortName, e.key) {
			return e.id
		}
	}
	return ""
}
