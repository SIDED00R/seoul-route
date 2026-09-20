package seoulbus

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 서울시 버스 API는 HTTP 엔드포인트를 사용한다.
const baseURL = "http://ws.bus.go.kr/api/rest/busRouteInfo"

// ErrQuota 는 공공데이터포털 일일 한도 초과(reason 22) 또는 키 거부다. 호출자는 중단하고 다음 날 재개한다.
var ErrQuota = errors.New("seoulbus: 일일 한도 초과 또는 키 거부")

// Client 는 응답을 cacheDir 에 JSON 으로 저장하고, 있으면 API 를 부르지 않는다.
type Client struct {
	Key      string
	CacheDir string
	HTTP     *http.Client
	MinGap   time.Duration // 연속 호출 최소 간격
	last     time.Time
}

func New(key, cacheDir string) *Client {
	return &Client{Key: key, CacheDir: cacheDir, HTTP: &http.Client{Timeout: 60 * time.Second},
		MinGap: 200 * time.Millisecond}
}

// AllRoutes 는 숫자별 노선 검색 결과를 합쳐 전체 노선을 만든다.
func (c *Client) AllRoutes() ([]Route, error) {
	seen := map[string]Route{}
	for _, d := range "0123456789" {
		var resp routeListResp
		q := url.Values{"strSrch": {string(d)}}
		if err := c.get("getBusRouteList", q, "routes_"+string(d)+".json", &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.MsgBody.ItemList {
			seen[r.ID] = r
		}
	}
	out := make([]Route, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// StopsByRoute 는 노선의 경유 정류장을 순서대로 돌려준다.
func (c *Client) StopsByRoute(routeID string) ([]Stop, error) {
	var resp stopListResp
	if err := c.get("getStaionByRoute", url.Values{"busRouteId": {routeID}}, "stops_"+routeID+".json", &resp); err != nil {
		return nil, err
	}
	stops := resp.MsgBody.ItemList
	sort.SliceStable(stops, func(i, j int) bool { return atoi(stops[i].Seq) < atoi(stops[j].Seq) })
	return stops, nil
}

// Cached 는 캐시 파일 존재 여부만 본다(fetch 진행률 계산용).
func (c *Client) Cached(name string) bool {
	_, err := os.Stat(filepath.Join(c.CacheDir, name))
	return err == nil
}

func (c *Client) get(op string, q url.Values, cacheName string, out any) error {
	path := filepath.Join(c.CacheDir, cacheName)
	if b, err := os.ReadFile(path); err == nil {
		return json.Unmarshal(b, out)
	}
	if wait := c.MinGap - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	q.Set("serviceKey", c.Key)
	q.Set("resultType", "json")
	resp, err := c.HTTP.Get(baseURL + "/" + op + "?" + q.Encode())
	c.last = time.Now()
	if err != nil {
		// *url.Error 의 Error() 는 키가 든 요청 URL 전체를 포함한다 → 원인(e.Err)만 래핑한다.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	// 게이트웨이 오류는 XML(OpenAPI_ServiceResponse) 로 온다. 키는 URL 에만 있으므로 본문 출력은 안전하다.
	if strings.Contains(string(body), "OpenAPI_ServiceResponse") {
		if strings.Contains(string(body), "LIMITED_NUMBER_OF_SERVICE_REQUESTS_EXCEEDS_ERROR") ||
			strings.Contains(string(body), "SERVICE_KEY_IS_NOT_REGISTERED_ERROR") ||
			strings.Contains(string(body), "SERVICE ACCESS DENIED") {
			return fmt.Errorf("%w: %s", ErrQuota, firstLine(body))
		}
		return fmt.Errorf("%s: 게이트웨이 오류: %s", op, firstLine(body))
	}
	var hdr struct {
		MsgHeader msgHeader `json:"msgHeader"`
	}
	if err := json.Unmarshal(body, &hdr); err != nil {
		return fmt.Errorf("%s: JSON 아님: %.120s", op, body)
	}
	if hdr.MsgHeader.HeaderCd != "0" && hdr.MsgHeader.HeaderCd != "4" { // 4 = 결과 없음
		return fmt.Errorf("%s: headerCd=%s %s", op, hdr.MsgHeader.HeaderCd, hdr.MsgHeader.HeaderMsg)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i > 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

func atoi(s string) int {
	n := 0
	for _, ch := range strings.TrimSpace(s) {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
