package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/geo"
)

// buildingRadiusM: 좌표를 품은 건물이 없을 때 이름을 빌려 올 건물까지의 상한(m, VWorld buffer). 아파트 단지 안 실내
// GPS 정지 표본 53개가 가장 가까운 건물 벽에서 중앙 5.0m·최대 22.1m, 현재 위치 버튼 좌표 7개가 0.9~8.3m(2026-09-29).
// 건물 사이 좌표에서 이름이 주소로 떨어지면 늘리고, 옆 건물 이름이 나오면 줄인다.
const buildingRadiusM = 30

// buildingTimeout: VWorld 건물 조회 한 번의 상한. 응답 실측 50~280ms(2026-09-29). 넘기면 buildingAt 은 빈 값이고
// 핸들러는 카카오 이름으로 답한다(앱은 /places/reverse 를 10초에 끊는다). 테스트에서 줄여 끼운다.
var buildingTimeout = 3 * time.Second

// vworldBuildings 는 VWorld 2D 데이터 API 도로명주소건물(LT_C_SPBD) 응답 중 쓰는 필드다.
type vworldBuildings struct {
	Response struct {
		Status string `json:"status"`
		Error  struct {
			Code string `json:"code"`
		} `json:"error"`
		Result struct {
			FeatureCollection struct {
				Features []struct {
					Geometry struct {
						Type        string          `json:"type"`
						Coordinates json.RawMessage `json:"coordinates"`
					} `json:"geometry"`
					Properties struct {
						Name   string `json:"buld_nm"`    // 건물명(예: "가나아파트")
						Detail string `json:"buld_nm_dc"` // 상세 건물명(예: "101동")
					} `json:"properties"`
				} `json:"features"`
			} `json:"featureCollection"`
		} `json:"result"`
	} `json:"response"`
}

// buildingAt 은 좌표를 품은 건물, 없으면 buildingRadiusM 안에서 가장 가까운 건물의 건물명과 상세 건물명
// (예: "가나아파트", "101동")을 돌려준다. 그 건물에 건물명이 없거나(상세 건물명만 있는 "1동" 포함), 건물이 없거나,
// 키가 없거나, 호출이 실패하면 빈 값 둘.
func (s *Server) buildingAt(ctx context.Context, lat, lon float64) (name, detail string) {
	if s.VWorldKey == "" {
		return "", ""
	}
	ctx, cancel := context.WithTimeout(ctx, buildingTimeout)
	defer cancel()
	params := url.Values{
		"service": {"data"}, "request": {"GetFeature"}, "data": {"LT_C_SPBD"}, "key": {s.VWorldKey},
		"format": {"json"}, "crs": {"EPSG:4326"}, "geometry": {"true"}, "size": {"30"},
		"geomFilter": {"POINT(" + strconv.FormatFloat(lon, 'f', 7, 64) + " " + strconv.FormatFloat(lat, 'f', 7, 64) + ")"},
		"buffer":     {strconv.Itoa(buildingRadiusM)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.vworldBase()+"/req/data?"+params.Encode(), nil)
	if err != nil {
		return "", ""
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		// url.Error 에는 키와 좌표가 든 요청 URL 이 실린다.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		s.Log.Warn("vworld building", "err", err)
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.Log.Warn("vworld building", "status", resp.StatusCode)
		return "", ""
	}
	var out vworldBuildings
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		s.Log.Warn("vworld building", "err", "응답 파싱 실패")
		return "", ""
	}
	// 반경 안에 건물이 없으면 OK 에 빈 목록 또는 NOT_FOUND 다.
	switch out.Response.Status {
	case "OK":
	case "NOT_FOUND":
		return "", ""
	default:
		s.Log.Warn("vworld building", "status", out.Response.Status, "code", out.Response.Error.Code)
		return "", ""
	}
	best := math.Inf(1)
	for _, f := range out.Response.Result.FeatureCollection.Features {
		for _, rings := range polygons(f.Geometry.Type, f.Geometry.Coordinates) {
			if d := geo.DistToPolygonM(lat, lon, rings); d < best {
				name, detail, best = f.Properties.Name, f.Properties.Detail, d
			}
		}
	}
	name, detail = strings.TrimSpace(name), strings.TrimSpace(detail)
	if name == "" {
		return "", ""
	}
	return name, detail
}

// polygons 는 GeoJSON Polygon·MultiPolygon 좌표를 다각형 목록으로 읽는다. 다른 형식이나 깨진 좌표면 nil.
func polygons(typ string, raw json.RawMessage) [][][][2]float64 {
	switch typ {
	case "Polygon":
		var p [][][2]float64
		if json.Unmarshal(raw, &p) == nil {
			return [][][][2]float64{p}
		}
	case "MultiPolygon":
		var mp [][][][2]float64
		if json.Unmarshal(raw, &mp) == nil {
			return mp
		}
	}
	return nil
}
