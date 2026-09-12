package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

// Place 는 앱에 내려주는 장소 검색 결과 한 건.
type Place struct {
	Name     string  `json:"name"`
	Address  string  `json:"address"`
	Category string  `json:"category,omitempty"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

// handlePlacesSearch 는 카카오 로컬 키워드 검색을 서울 bbox 로 한정해 대신 호출한다.
// 카카오 REST 키는 서버 .env 에만 두므로 앱은 이 경로를 통해서만 검색한다.
func (s *Server) handlePlacesSearch(w http.ResponseWriter, r *http.Request) {
	if s.KakaoKey == "" {
		writeError(w, http.StatusServiceUnavailable, "장소 검색 미설정")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || utf8.RuneCountInString(q) > 100 {
		writeError(w, http.StatusBadRequest, "q 는 1~100자")
		return
	}
	params := url.Values{"query": {q}, "size": {"10"},
		"rect": {fmt.Sprintf("%.2f,%.2f,%.2f,%.2f", route.MinLon, route.MinLat, route.MaxLon, route.MaxLat)}}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		"https://dapi.kakao.com/v2/local/search/keyword.json?"+params.Encode(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "요청 생성 실패")
		return
	}
	req.Header.Set("Authorization", "KakaoAK "+s.KakaoKey)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		// url.Error 는 요청 URL 을 포함하지만 키는 헤더에 있어 로그에 남지 않는다.
		s.Log.Warn("kakao search", "err", err)
		writeError(w, http.StatusBadGateway, "장소 검색 실패")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.Log.Warn("kakao search", "status", resp.StatusCode)
		writeError(w, http.StatusBadGateway, "장소 검색 실패")
		return
	}
	var out struct {
		Documents []struct {
			PlaceName   string `json:"place_name"`
			RoadAddress string `json:"road_address_name"`
			Address     string `json:"address_name"`
			Category    string `json:"category_group_name"`
			X           string `json:"x"`
			Y           string `json:"y"`
		} `json:"documents"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		writeError(w, http.StatusBadGateway, "장소 검색 응답 파싱 실패")
		return
	}
	places := make([]Place, 0, len(out.Documents))
	for _, d := range out.Documents {
		lon, errX := strconv.ParseFloat(d.X, 64)
		lat, errY := strconv.ParseFloat(d.Y, 64)
		if errX != nil || errY != nil {
			continue
		}
		addr := d.RoadAddress
		if addr == "" {
			addr = d.Address
		}
		places = append(places, Place{Name: d.PlaceName, Address: addr, Category: d.Category, Lat: lat, Lon: lon})
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": places})
}
