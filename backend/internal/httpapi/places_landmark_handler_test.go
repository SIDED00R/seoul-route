package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPlacesLandmarkChoosesNearestAndCaches(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	kakao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		q := r.URL.Query()
		if q.Get("radius") != "30" || q.Get("sort") != "distance" || q.Get("size") != "1" ||
			q.Get("x") == "" || q.Get("y") == "" {
			t.Errorf("query=%v", q)
		}
		switch q.Get("category_group_code") {
		case "BK9":
			io.WriteString(w, `{"documents":[{"place_name":"우리은행 목동점","category_group_name":"은행",`+
				`"x":"126.8721","y":"37.5301","distance":"8"}]}`)
		case "SW8":
			io.WriteString(w, `{"documents":[{"place_name":"오목교역","category_group_name":"지하철역",`+
				`"x":"126.8720","y":"37.5300","distance":"15"}]}`)
		default:
			io.WriteString(w, `{"documents":[]}`)
		}
	}))
	defer kakao.Close()
	s := &Server{KakaoKey: "k", KakaoBase: kakao.URL, HTTP: kakao.Client(), Log: slog.New(slog.DiscardHandler)}

	request := func() map[string]any {
		rr := httptest.NewRecorder()
		s.handlePlacesLandmark(rr, httptest.NewRequest(http.MethodGet,
			"/places/landmark?lat=37.5300&lon=126.8720", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("code=%d body=%s", rr.Code, rr.Body)
		}
		var out map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := request()
	lm := out["landmark"].(map[string]any)
	if lm["name"] != "우리은행 목동점" || lm["distance_m"] != float64(8) {
		t.Fatalf("landmark=%v", lm)
	}
	request()
	mu.Lock()
	defer mu.Unlock()
	if calls != len(landmarkCategories) {
		t.Errorf("캐시 뒤 calls=%d want=%d", calls, len(landmarkCategories))
	}
}

// 일부 카테고리가 실패한 결과는 24시간이 아니라 landmarkPartialTTL 만 캐시된다 — 만료 뒤 요청은 카카오를 다시 부른다.
func TestPlacesLandmarkPartialFailureCachesBriefly(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	kakao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		switch r.URL.Query().Get("category_group_code") {
		case "BK9":
			w.WriteHeader(http.StatusTooManyRequests) // 은행 카테고리만 순간 제한
		case "MT1":
			io.WriteString(w, `{"documents":[{"place_name":"이마트","category_group_name":"대형마트",`+
				`"x":"126.8720","y":"37.5300","distance":"28"}]}`)
		default:
			io.WriteString(w, `{"documents":[]}`)
		}
	}))
	defer kakao.Close()
	s := &Server{KakaoKey: "k", KakaoBase: kakao.URL, HTTP: kakao.Client(), Log: slog.New(slog.DiscardHandler)}
	req := func() *http.Request {
		return httptest.NewRequest(http.MethodGet, "/places/landmark?lat=37.5300&lon=126.8720", nil)
	}
	rr := httptest.NewRecorder()
	s.handlePlacesLandmark(rr, req())
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "이마트") {
		t.Fatalf("부분 성공은 남은 결과로 200 이어야 한다: code=%d body=%s", rr.Code, rr.Body)
	}
	s.landmarkMu.Lock()
	c, ok := s.landmarks["37.5300,126.8720"]
	s.landmarkMu.Unlock()
	if !ok {
		t.Fatal("부분 성공도 캐시돼야 한다(제한 중 재호출 폭주 방지)")
	}
	remaining := time.Until(c.expires)
	if remaining > landmarkPartialTTL || remaining < landmarkPartialTTL-10*time.Second {
		t.Fatalf("부분 성공 TTL=%v want≈%v", remaining, landmarkPartialTTL)
	}
	// 만료시키면 다음 요청이 다시 카카오를 부른다(전체 성공 24시간 캐시와 다른 점).
	s.landmarkMu.Lock()
	c.expires = time.Now().Add(-time.Second)
	s.landmarks["37.5300,126.8720"] = c
	s.landmarkMu.Unlock()
	s.handlePlacesLandmark(httptest.NewRecorder(), req())
	mu.Lock()
	defer mu.Unlock()
	if calls != 2*len(landmarkCategories) {
		t.Fatalf("만료 뒤 재호출 calls=%d want=%d", calls, 2*len(landmarkCategories))
	}
}

func TestLandmarkCacheDropsExpiredAndStaysBounded(t *testing.T) {
	now := time.Now()
	s := &Server{landmarks: make(map[string]cachedLandmark)}
	s.landmarks["expired"] = cachedLandmark{expires: now.Add(-time.Minute)}
	for i := 0; i < landmarkCacheLimit; i++ {
		s.landmarks[fmt.Sprintf("key-%d", i)] = cachedLandmark{expires: now.Add(time.Hour)}
	}
	s.cacheLandmark("new", nil, time.Hour)
	if _, ok := s.landmarks["expired"]; ok {
		t.Error("만료 항목이 남았다")
	}
	if got := len(s.landmarks); got != landmarkCacheLimit {
		t.Fatalf("cache size=%d want=%d", got, landmarkCacheLimit)
	}
}

func TestPlacesLandmarkErrors(t *testing.T) {
	s := &Server{KakaoKey: "k", HTTP: http.DefaultClient, Log: slog.New(slog.DiscardHandler)}
	for _, query := range []string{"", "?lat=x&lon=126.9", "?lat=35.1&lon=129", "?lat=NaN&lon=126.9"} {
		rr := httptest.NewRecorder()
		s.handlePlacesLandmark(rr, httptest.NewRequest(http.MethodGet, "/places/landmark"+query, nil))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("query=%q code=%d body=%s", query, rr.Code, rr.Body)
		}
	}
	noKey := &Server{HTTP: http.DefaultClient, Log: slog.New(slog.DiscardHandler)}
	rr := httptest.NewRecorder()
	noKey.handlePlacesLandmark(rr, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/places/landmark?lat=%v&lon=%v", 37.53, 126.87), nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("키 없음 code=%d", rr.Code)
	}
}
