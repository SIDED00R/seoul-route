// Package realtime 은 검색 결과의 첫 대중교통 탑승 대기를 실시간 도착정보로 바꾼다.
// 예측은 시간표(OTP)로 하고 실시간은 후보가 나온 뒤 재계산·순위 조정에만 쓴다(제품 결정 2026-09-12).
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

// CacheTTL: 같은 노선·역의 도착정보를 이 시간 안에는 다시 부르지 않는다. 개발계정 하루 1,000콜 한도 대비.
const CacheTTL = 30 * time.Second

// BusArrival 은 한 정류장의 다음 차 도착 예정(초). 값이 없으면 nil.
type BusArrival struct {
	ExpsSec []int // 첫 차, 둘째 차 순. "출발대기"·"운행종료" 는 제외. 캐시 경과시간을 뺀 값이라 음수(이미 지나감)일 수 있다
}

// BusClient 는 서울 버스 도착정보 getArrInfoByRouteAll(노선 전체 정류장) 클라이언트.
// 노선당 1콜로 모든 정류장의 exps1/exps2 를 받아 캐시한다.
type BusClient struct {
	Key  string
	HTTP *http.Client
	Base string // 기본 http://ws.bus.go.kr (443 미개방)

	mu    sync.Mutex
	cache map[string]busEntry // busRouteId → 정류장별 도착
}

type busEntry struct {
	at    time.Time
	stops map[string]BusArrival // stId → 도착
}

// Arrival 은 노선 routeID(서울 busRouteId) 의 정류장 stID 다음 차 도착 예정을 돌려준다.
func (c *BusClient) Arrival(ctx context.Context, routeID, stID string) (BusArrival, bool, error) {
	c.mu.Lock()
	e, ok := c.cache[routeID]
	c.mu.Unlock()
	if !ok || time.Since(e.at) > CacheTTL {
		stops, err := c.fetch(ctx, routeID)
		if err != nil {
			return BusArrival{}, false, err
		}
		e = busEntry{at: time.Now(), stops: stops}
		c.mu.Lock()
		if c.cache == nil {
			c.cache = map[string]busEntry{}
		}
		c.cache[routeID] = e
		c.mu.Unlock()
	}
	a, ok := e.stops[stID]
	if !ok {
		return BusArrival{}, false, nil
	}
	// 캐시의 ETA 는 조회 시각 기준 상대값이다. 지금 기준으로 쓰려면 경과시간을 뺀다(사본에 — 캐시는 그대로).
	age := int(time.Since(e.at).Seconds())
	out := BusArrival{ExpsSec: make([]int, len(a.ExpsSec))}
	for i, s := range a.ExpsSec {
		out.ExpsSec[i] = s - age
	}
	return out, true, nil
}

func (c *BusClient) fetch(ctx context.Context, routeID string) (map[string]BusArrival, error) {
	base := c.Base
	if base == "" {
		base = "http://ws.bus.go.kr"
	}
	q := url.Values{"serviceKey": {c.Key}, "busRouteId": {routeID}, "resultType": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/api/rest/arrive/getArrInfoByRouteAll?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// url.Error 에 키가 든 URL 이 실리므로 원문은 남기지 않는다.
		return nil, fmt.Errorf("bus arrival: 요청 실패")
	}
	defer resp.Body.Close()
	var out struct {
		MsgHeader struct {
			HeaderCd  string `json:"headerCd"`
			HeaderMsg string `json:"headerMsg"`
		} `json:"msgHeader"`
		MsgBody struct {
			ItemList []struct {
				StID    string `json:"stId"`
				Arrmsg1 string `json:"arrmsg1"`
				Arrmsg2 string `json:"arrmsg2"`
				Exps1   string `json:"exps1"`
				Exps2   string `json:"exps2"`
			} `json:"itemList"`
		} `json:"msgBody"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("bus arrival: 응답 파싱 실패 (HTTP %d)", resp.StatusCode)
	}
	if out.MsgHeader.HeaderCd != "0" {
		return nil, fmt.Errorf("bus arrival: %s (%s)", out.MsgHeader.HeaderMsg, out.MsgHeader.HeaderCd)
	}
	stops := make(map[string]BusArrival, len(out.MsgBody.ItemList))
	for _, it := range out.MsgBody.ItemList {
		var a BusArrival
		for _, pair := range [][2]string{{it.Arrmsg1, it.Exps1}, {it.Arrmsg2, it.Exps2}} {
			if s, ok := busETA(pair[0], pair[1]); ok {
				a.ExpsSec = append(a.ExpsSec, s)
			}
		}
		stops[it.StID] = a
	}
	return stops, nil
}

// busETA 는 arrmsg("3분12초후[2번째 전]", "곧 도착", "출발대기", "운행종료")와 exps(초)로 도착 초를 정한다.
// 출발대기·운행종료·빈 값은 예측 없음. "곧 도착" 은 exps 가 0 이어도 0초로 본다.
func busETA(msg, exps string) (int, bool) {
	msg = strings.TrimSpace(msg)
	if msg == "" || strings.Contains(msg, "출발대기") || strings.Contains(msg, "운행종료") {
		return 0, false
	}
	s, err := strconv.Atoi(strings.TrimSpace(exps))
	if err != nil || s < 0 {
		return 0, false
	}
	return s, true
}
