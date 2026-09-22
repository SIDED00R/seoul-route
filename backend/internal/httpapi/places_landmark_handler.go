package httpapi

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

const (
	landmarkRadiusM    = 30
	landmarkCacheLimit = 2048
)

var landmarkCategories = []string{"SW8", "SC4", "BK9", "PO3", "HP8", "MT1"}

type landmark struct {
	Name      string  `json:"name"`
	Category  string  `json:"category"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	DistanceM int     `json:"distance_m"`
}

type cachedLandmark struct {
	value   *landmark
	expires time.Time
}

type landmarkResult struct {
	value *landmark
	ok    bool
}

// handlePlacesLandmark 는 회전점에서 눈에 띄는 시설을 찾는다. 경로 탐색과 분리해 실패해도 안내가 거리 문구로
// 계속될 수 있게 한다. 좌표는 약 10m 격자로 캐시해 인접한 OTP 단계가 카카오를 반복 호출하지 않게 한다.
func (s *Server) handlePlacesLandmark(w http.ResponseWriter, r *http.Request) {
	if s.KakaoKey == "" {
		writeError(w, http.StatusServiceUnavailable, "장소 검색 미설정")
		return
	}
	lat, errLat := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, errLon := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if errLat != nil || errLon != nil || math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) ||
		lat < route.MinLat || lat > route.MaxLat || lon < route.MinLon || lon > route.MaxLon {
		writeError(w, http.StatusBadRequest, "서울 범위의 lat·lon 이 필요하다")
		return
	}
	key := fmt.Sprintf("%.4f,%.4f", lat, lon)
	s.landmarkMu.Lock()
	if c, ok := s.landmarks[key]; ok && time.Now().Before(c.expires) {
		s.landmarkMu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"landmark": c.value})
		return
	}
	s.landmarkMu.Unlock()

	results := make(chan landmarkResult, len(landmarkCategories))
	var wg sync.WaitGroup
	for _, category := range landmarkCategories {
		wg.Add(1)
		go func(category string) {
			defer wg.Done()
			results <- s.fetchLandmarkCategory(r.Context(), category, lat, lon)
		}(category)
	}
	wg.Wait()
	close(results)
	var best *landmark
	succeeded := false
	for result := range results {
		if !result.ok {
			continue
		}
		succeeded = true
		if result.value != nil && (best == nil || result.value.DistanceM < best.DistanceM) {
			best = result.value
		}
	}
	if !succeeded {
		writeError(w, http.StatusBadGateway, "랜드마크 조회 실패")
		return
	}
	ttl := time.Hour
	if best != nil {
		ttl = 24 * time.Hour
	}
	s.cacheLandmark(key, best, ttl)
	writeJSON(w, http.StatusOK, map[string]any{"landmark": best})
}

func (s *Server) cacheLandmark(key string, value *landmark, ttl time.Duration) {
	now := time.Now()
	s.landmarkMu.Lock()
	defer s.landmarkMu.Unlock()
	if s.landmarks == nil {
		s.landmarks = make(map[string]cachedLandmark)
	}
	for cachedKey, cached := range s.landmarks {
		if !now.Before(cached.expires) {
			delete(s.landmarks, cachedKey)
		}
	}
	if _, exists := s.landmarks[key]; !exists && len(s.landmarks) >= landmarkCacheLimit {
		var oldestKey string
		var oldestExpiry time.Time
		for cachedKey, cached := range s.landmarks {
			if oldestKey == "" || cached.expires.Before(oldestExpiry) {
				oldestKey = cachedKey
				oldestExpiry = cached.expires
			}
		}
		delete(s.landmarks, oldestKey)
	}
	s.landmarks[key] = cachedLandmark{value: value, expires: now.Add(ttl)}
}

func (s *Server) fetchLandmarkCategory(ctx context.Context, category string, lat, lon float64) landmarkResult {
	params := url.Values{
		"category_group_code": {category},
		"x":                   {strconv.FormatFloat(lon, 'f', 7, 64)},
		"y":                   {strconv.FormatFloat(lat, 'f', 7, 64)},
		"radius":              {strconv.Itoa(landmarkRadiusM)},
		"sort":                {"distance"},
		"size":                {"1"},
	}
	var out struct {
		Documents []struct {
			Name     string `json:"place_name"`
			Category string `json:"category_group_name"`
			X        string `json:"x"`
			Y        string `json:"y"`
			Distance string `json:"distance"`
		} `json:"documents"`
	}
	if s.kakaoGet(ctx, "kakao landmark", "/v2/local/search/category.json", params, &out) != kakaoOK {
		return landmarkResult{}
	}
	if len(out.Documents) == 0 {
		return landmarkResult{ok: true}
	}
	d := out.Documents[0]
	x, errX := strconv.ParseFloat(d.X, 64)
	y, errY := strconv.ParseFloat(d.Y, 64)
	distance, errD := strconv.Atoi(d.Distance)
	if d.Name == "" || errX != nil || errY != nil || errD != nil || distance > landmarkRadiusM {
		return landmarkResult{ok: true}
	}
	return landmarkResult{ok: true, value: &landmark{Name: d.Name, Category: d.Category, Lat: y, Lon: x, DistanceM: distance}}
}
