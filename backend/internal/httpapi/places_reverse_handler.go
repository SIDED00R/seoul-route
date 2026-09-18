package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"

	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

// KakaoBaseURL 은 카카오 로컬 API 주소. 테스트에서 Server.KakaoBase 로 바꿔 끼운다.
const KakaoBaseURL = "https://dapi.kakao.com"

// handlePlacesReverse 는 좌표를 주소·건물 이름으로 바꿔 준다(카카오 로컬 좌표→주소).
// 앱의 "현재 위치" 출발지 표시에 쓴다. 좌표는 요청 경로가 아니라 쿼리로 받으므로 접근 로그에 남지 않는다.
func (s *Server) handlePlacesReverse(w http.ResponseWriter, r *http.Request) {
	if s.KakaoKey == "" {
		writeError(w, http.StatusServiceUnavailable, "장소 검색 미설정")
		return
	}
	lat, errLat := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, errLon := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	// NaN 은 어떤 비교에도 false 라 범위 검사를 그냥 지나간다(무한대는 범위 검사에 걸린다).
	if errLat != nil || errLon != nil || math.IsNaN(lat) || math.IsNaN(lon) ||
		lat < route.MinLat || lat > route.MaxLat || lon < route.MinLon || lon > route.MaxLon {
		writeError(w, http.StatusBadRequest, "서울 범위의 lat·lon 이 필요하다")
		return
	}
	params := url.Values{"x": {strconv.FormatFloat(lon, 'f', 7, 64)}, "y": {strconv.FormatFloat(lat, 'f', 7, 64)}}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		s.kakaoBase()+"/v2/local/geo/coord2address.json?"+params.Encode(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "요청 생성 실패")
		return
	}
	req.Header.Set("Authorization", "KakaoAK "+s.KakaoKey)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		// url.Error 에는 좌표가 든 요청 URL 이 실리므로 그 껍질을 벗기고 원인만 남긴다(dial·timeout·취소).
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		s.Log.Warn("kakao coord2address", "err", err)
		writeError(w, http.StatusBadGateway, "주소 조회 실패")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.Log.Warn("kakao coord2address", "status", resp.StatusCode)
		writeError(w, http.StatusBadGateway, "주소 조회 실패")
		return
	}
	var out struct {
		Documents []struct {
			RoadAddress *struct {
				BuildingName string `json:"building_name"`
				AddressName  string `json:"address_name"`
			} `json:"road_address"`
			Address *struct {
				AddressName string `json:"address_name"`
			} `json:"address"`
		} `json:"documents"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		writeError(w, http.StatusBadGateway, "주소 응답 파싱 실패")
		return
	}
	name, address := "", ""
	if len(out.Documents) > 0 {
		d := out.Documents[0]
		if d.RoadAddress != nil {
			name, address = d.RoadAddress.BuildingName, d.RoadAddress.AddressName
		}
		if address == "" && d.Address != nil {
			address = d.Address.AddressName
		}
		// 건물 이름은 도로명주소 대장에 있는 좌표에만 있다. 없으면 주소를 이름으로 쓴다.
		if name == "" {
			name = address
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "address": address})
}

func (s *Server) kakaoBase() string {
	if s.KakaoBase != "" {
		return s.KakaoBase
	}
	return KakaoBaseURL
}
