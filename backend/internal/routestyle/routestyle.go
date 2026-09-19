// Package routestyle 은 생성 GTFS zip 의 routes.txt 에서 노선 색을 읽는다.
// OTP 는 graph.obj 에 담긴 옛 노선 정보를 쓰므로 색을 여기서 직접 읽어 leg 에 붙인다(그래프 재빌드가 필요 없다).
package routestyle

import (
	"strings"

	"github.com/SIDED00R/seoul-route/backend/internal/gtfszip"
)

// Style 은 한 노선의 색. 값은 GTFS route_color 형식(# 없는 6자리 16진수)이고, 비어 있으면 앱이 기본 팔레트를 쓴다.
type Style struct {
	Color     string
	TextColor string
}

// Load 는 route_id → 색을 돌려준다. 색이 없는 노선은 넣지 않는다. routes.txt 가 없으면 색 없이 진행한다.
func Load(zipPath string) (map[string]Style, error) {
	zr, err := gtfszip.Open(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	rows, _, err := zr.Rows("routes.txt")
	if err != nil {
		return nil, err
	}
	out := map[string]Style{}
	for _, r := range rows {
		color := strings.TrimSpace(r["route_color"])
		if color == "" {
			continue
		}
		out[r["route_id"]] = Style{Color: color, TextColor: strings.TrimSpace(r["route_text_color"])}
	}
	return out, nil
}
