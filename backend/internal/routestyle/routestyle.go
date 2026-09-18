// Package routestyle 은 생성 GTFS zip 의 routes.txt 에서 노선 색을 읽는다.
// OTP 는 graph.obj 에 담긴 옛 노선 정보를 쓰므로 색을 여기서 직접 읽어 leg 에 붙인다(그래프 재빌드가 필요 없다).
package routestyle

import (
	"archive/zip"
	"encoding/csv"
	"io"
	"strings"
)

// Style 은 한 노선의 색. 값은 GTFS route_color 형식(# 없는 6자리 16진수)이고, 비어 있으면 앱이 기본 팔레트를 쓴다.
type Style struct {
	Color     string
	TextColor string
}

// Load 는 route_id → 색을 돌려준다. 색이 없는 노선은 넣지 않는다.
func Load(zipPath string) (map[string]Style, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	rows, err := readRoutes(zr)
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

func readRoutes(zr *zip.ReadCloser) ([]map[string]string, error) {
	for _, f := range zr.File {
		if f.Name != "routes.txt" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		cr := csv.NewReader(rc)
		header, err := cr.Read()
		if err != nil {
			return nil, err
		}
		header[0] = strings.TrimPrefix(header[0], "\xef\xbb\xbf")
		var rows []map[string]string
		for {
			rec, err := cr.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			row := map[string]string{}
			for i, h := range header {
				if i < len(rec) {
					row[h] = rec[i]
				}
			}
			rows = append(rows, row)
		}
		return rows, nil
	}
	return nil, nil // routes.txt 가 없으면 색 없이 진행한다
}
