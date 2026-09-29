package httpapi

import (
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

// handlePlacesReverse 는 좌표를 이름·주소로 바꿔 준다. 앱의 "현재 위치" 출발지 표시에 쓴다.
// 이름은 사용자 즐겨찾기(favoriteNear) > 카카오 건물 이름(같은 단지면 VWorld 건물의 동으로 바꿔 끼움, buildingAt)
// > VWorld 건물 이름 > 주소 순이고, 주소는 카카오 좌표→주소다. 카카오 건물 이름은 좌표가 건물 윤곽 밖(건물 사이)이면
// 지번 필지의 대표 건물이라, 아파트 단지에서는 옆 동이 아닌 대표 동이 나온다.
// 좌표는 요청 경로가 아니라 쿼리로 받으므로 접근 로그에 남지 않는다.
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
	switch s.kakaoGet(r.Context(), "kakao coord2address", "/v2/local/geo/coord2address.json", params, &out) {
	case kakaoBadRequest:
		writeError(w, http.StatusInternalServerError, "요청 생성 실패")
		return
	case kakaoCallFailed:
		writeError(w, http.StatusBadGateway, "주소 조회 실패")
		return
	case kakaoBadBody:
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
	}
	// 라우터에서는 requireAuth 뒤라 늘 사용자가 있다. 핸들러를 바로 부르는 단위 테스트에는 없다.
	fav := ""
	if uid := userIDFrom(r.Context()); uid != "" {
		fav = s.favoriteNear(r.Context(), uid, lat, lon)
	}
	if fav != "" {
		name = fav
	} else if bld, detail := s.buildingAt(r.Context(), lat, lon); bld != "" {
		switch {
		case name == "":
			name = strings.TrimSpace(bld + " " + detail)
		case detail != "" && strings.HasPrefix(name, bld):
			// 카카오 이름이 같은 단지의 대표 동("가나아파트 105동")이면 좌표가 든 동으로 바꾼다.
			name = bld + " " + detail
		}
		// 그 밖에는 카카오 이름을 둔다(같은 건물이 카카오 "코엑스", VWorld "ASEM 및 한국종합무역센타단지 컨벤션센터").
	}
	// 건물 이름은 도로명주소 대장에 있는 좌표에만 있다. 없으면 주소를 이름으로 쓴다.
	if name == "" {
		name = address
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "address": address})
}
