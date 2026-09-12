package gbfs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// 가짜 열린데이터광장: 1페이지 1000행, 2페이지 3행 → 짧은 페이지에서 멈춰야 한다. list_total_count 는 일부러 엉뚱한 값.
func fakeUpstream(t *testing.T, fail *bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if *fail {
			w.WriteHeader(500)
			return
		}
		var start, end int
		fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/testkey/json/bikeList/"), "%d/%d/", &start, &end)
		n := PageSize
		if start > PageSize {
			n = 3
		}
		rows := make([]map[string]string, 0, n)
		for i := 0; i < n; i++ {
			id := start + i
			rows = append(rows, map[string]string{"stationId": fmt.Sprintf("ST-%d", id), "stationName": "s",
				"stationLatitude": "37.55", "stationLongitude": "126.97", "rackTotCnt": "10",
				"parkingBikeTotCnt": fmt.Sprintf("%d", id%12)})
		}
		json.NewEncoder(w).Encode(map[string]any{"rentBikeStatus": map[string]any{
			"list_total_count": 5, "RESULT": map[string]string{"CODE": "INFO-000"}, "row": rows}})
	}))
}

func TestPollerPagesUntilShortPageAndServesGBFS(t *testing.T) {
	fail := false
	up := fakeUpstream(t, &fail)
	defer up.Close()
	p := NewPoller("testkey", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	p.BaseURL = up.URL
	p.pollOnce(context.Background())
	snap, stale := p.Current()
	if snap == nil || stale || len(snap.Stations) != PageSize+3 {
		t.Fatalf("snap=%v stale=%v", snap, stale)
	}
	h := &Handler{Poller: p, BaseURL: "http://api/gbfs"}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/gbfs/station_status.json", nil))
	var out struct {
		Data struct {
			Stations []struct {
				ID    string `json:"station_id"`
				Bikes int    `json:"num_bikes_available"`
				Docks int    `json:"num_docks_available"`
				Rent  bool   `json:"is_renting"`
			} `json:"stations"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &out)
	if rr.Code != 200 || len(out.Data.Stations) != PageSize+3 || !out.Data.Stations[0].Rent {
		t.Fatalf("status code=%d n=%d", rr.Code, len(out.Data.Stations))
	}
	// ST-1: 1%12=1 대 → docks 9. ST-11: 11 대 → docks 0(10-11 → 음수는 0)
	for _, s := range out.Data.Stations {
		if s.ID == "ST-1" && (s.Bikes != 1 || s.Docks != 9) {
			t.Fatalf("ST-1 %+v", s)
		}
		if s.ID == "ST-11" && (s.Bikes != 11 || s.Docks != 0) {
			t.Fatalf("ST-11 %+v", s)
		}
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/gbfs/gbfs.json", nil))
	if !strings.Contains(rr.Body.String(), `"http://api/gbfs/station_status.json"`) {
		t.Fatalf("gbfs.json feeds: %s", rr.Body.String())
	}
}

// 총수가 정확히 PageSize 의 배수면 마지막 페이지가 꽉 차고 다음 요청은 평면 INFO-200(실측 본문)이다 → 빈 페이지로 끝나야 한다.
func TestPollerHandlesExactMultipleOfPageSize(t *testing.T) {
	total := 2 * PageSize
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int
		fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/testkey/json/bikeList/"), "%d/%d/", &start, &end)
		if start > total {
			w.Write([]byte(`{"CODE":"INFO-200","MESSAGE":"해당하는 데이터가 없습니다."}`))
			return
		}
		rows := make([]map[string]string, 0, PageSize)
		for id := start; id <= end && id <= total; id++ {
			rows = append(rows, map[string]string{"stationId": fmt.Sprintf("ST-%d", id), "stationName": "s",
				"stationLatitude": "37.55", "stationLongitude": "126.97", "rackTotCnt": "10", "parkingBikeTotCnt": "1"})
		}
		json.NewEncoder(w).Encode(map[string]any{"rentBikeStatus": map[string]any{
			"list_total_count": len(rows), "RESULT": map[string]string{"CODE": "INFO-000"}, "row": rows}})
	}))
	defer up.Close()
	p := NewPoller("testkey", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	p.BaseURL = up.URL
	stations, err := p.fetchAll(context.Background())
	if err != nil || len(stations) != total {
		t.Fatalf("n=%d err=%v (INFO-200 을 오류로 보면 수집분 전량이 버려진다)", len(stations), err)
	}
}

// 상류가 실패하면 마지막 스냅샷을 유지하되, MaxAge 를 넘기면 stale → is_renting=false.
func TestPollerKeepsLastKnownGoodAndMarksStale(t *testing.T) {
	fail := false
	up := fakeUpstream(t, &fail)
	defer up.Close()
	p := NewPoller("testkey", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	p.BaseURL = up.URL
	now := time.Now()
	p.now = func() time.Time { return now }
	p.pollOnce(context.Background())
	fail = true
	p.pollOnce(context.Background())
	if snap, stale := p.Current(); snap == nil || stale {
		t.Fatalf("실패 직후에도 마지막 정상 스냅샷을 내야 한다: %v %v", snap, stale)
	}
	now = now.Add(MaxAge + time.Minute)
	_, stale := p.Current()
	if !stale {
		t.Fatal("MaxAge 초과면 stale 이어야 한다")
	}
	rr := httptest.NewRecorder()
	h := &Handler{Poller: p, BaseURL: "http://api/gbfs"}
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/gbfs/station_status.json", nil))
	if rr.Header().Get("X-GBFS-Stale") != "true" || !strings.Contains(rr.Body.String(), `"is_renting":false`) {
		t.Fatalf("stale 응답: %s %s", rr.Header().Get("X-GBFS-Stale"), rr.Body.String()[:120])
	}
}
