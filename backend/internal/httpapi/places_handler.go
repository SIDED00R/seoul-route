package httpapi

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

// stationTimeout: 역 검색(보조 호출) 상한. 카카오 키워드 검색 응답 실측 49~130ms(2026-09-29). 넘기면 본 검색 순서로
// 답한다(앱은 /places/search 를 15초에 끊는다). 테스트에서 줄여 끼운다.
var stationTimeout = 3 * time.Second

// Place 는 앱에 내려주는 장소 검색 결과 한 건.
type Place struct {
	Name      string  `json:"name"`
	Address   string  `json:"address"`
	Category  string  `json:"category,omitempty"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	DistanceM int     `json:"distance_m,omitempty"` // 검색 요청에 위치가 있을 때 그 위치에서 직선거리
}

// handlePlacesSearch 는 카카오 로컬 키워드 검색을 서울 bbox 로 한정해 대신 호출하고 길찾기 용도 순서로 다시 세운다
// (rankPlaces). lat·lon(선택)은 사용자 위치로 카카오에 x·y 로 넘긴다: 카카오 정확도순은 그때 체인·업종 검색
// ("스타벅스"·"약국")을 가까운 곳부터 세우고(지역·명소 검색 "강남"·"홍대" 순서는 그대로, 2026-09-29 실측) 거리를 붙인다.
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
	at, ok := searchLocation(r.URL.Query())
	if !ok {
		writeError(w, http.StatusBadRequest, "lat·lon 은 둘 다 숫자여야 한다")
		return
	}
	rect := fmt.Sprintf("%.2f,%.2f,%.2f,%.2f", route.MinLon, route.MinLat, route.MaxLon, route.MaxLat)
	var main kakaoKeywordResult
	switch s.kakaoGet(r.Context(), "kakao search", "/v2/local/search/keyword.json",
		at.with(url.Values{"query": {q}, "size": {"15"}, "rect": {rect}}), &main) {
	case kakaoBadRequest:
		writeError(w, http.StatusInternalServerError, "요청 생성 실패")
		return
	case kakaoCallFailed:
		writeError(w, http.StatusBadGateway, "장소 검색 실패")
		return
	case kakaoBadBody:
		writeError(w, http.StatusBadGateway, "장소 검색 응답 파싱 실패")
		return
	}
	// 역 검색은 순서만 보탠다. 실패하거나 stationTimeout 안에 답하지 않으면 없는 셈 치고 카카오 순서로 답한다.
	var stations kakaoKeywordResult
	if needsStationBoost(q, main) {
		ctx, cancel := context.WithTimeout(r.Context(), stationTimeout)
		s.kakaoGet(ctx, "kakao search stations", "/v2/local/search/keyword.json",
			at.with(url.Values{"query": {q + "역"}, "category_group_code": {"SW8"}, "size": {"15"}, "rect": {rect}}), &stations)
		cancel()
	}
	docs := rankPlaces(q, main, stations.Documents)
	places := make([]Place, 0, len(docs))
	for _, d := range docs {
		lon, errX := strconv.ParseFloat(d.X, 64)
		lat, errY := strconv.ParseFloat(d.Y, 64)
		if errX != nil || errY != nil {
			continue
		}
		addr := d.RoadAddress
		if addr == "" {
			addr = d.Address
		}
		dist, _ := strconv.Atoi(d.Distance)
		places = append(places,
			Place{Name: d.PlaceName, Address: addr, Category: d.Category, Lat: lat, Lon: lon, DistanceM: dist})
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": places})
}

// searchPoint 는 검색 요청의 사용자 위치다. ok 가 false 면 위치 없이 검색한다.
type searchPoint struct {
	lat, lon float64
	ok       bool
}

// with 는 위치가 있으면 카카오 요청에 x(경도)·y(위도)를 더한다.
func (p searchPoint) with(v url.Values) url.Values {
	if p.ok {
		v.Set("x", strconv.FormatFloat(p.lon, 'f', 7, 64))
		v.Set("y", strconv.FormatFloat(p.lat, 'f', 7, 64))
	}
	return v
}

// searchLocation 은 lat·lon 쿼리를 읽는다. 둘 다 없으면 위치 없음, 하나만 있거나 숫자가 아니면 ok=false(400).
// 서울 bbox 밖 위치는 위치 없음으로 본다(검색 대상은 bbox 안이고, 서울 밖에서도 검색은 된다).
func searchLocation(v url.Values) (searchPoint, bool) {
	latStr, lonStr := v.Get("lat"), v.Get("lon")
	if latStr == "" && lonStr == "" {
		return searchPoint{}, true
	}
	lat, errLat := strconv.ParseFloat(latStr, 64)
	lon, errLon := strconv.ParseFloat(lonStr, 64)
	if errLat != nil || errLon != nil || math.IsNaN(lat) || math.IsNaN(lon) {
		return searchPoint{}, false
	}
	if lat < route.MinLat || lat > route.MaxLat || lon < route.MinLon || lon > route.MaxLon {
		return searchPoint{}, true
	}
	return searchPoint{lat: lat, lon: lon, ok: true}, true
}
