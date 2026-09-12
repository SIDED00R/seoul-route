// Package headway 는 생성 GTFS zip 의 frequencies.txt 에서 노선별 배차간격을 읽는다.
// 버스는 배차간격 기반 시간표라 OTP 의 previousLegs/nextLegs 가 쓸 값을 주지 않는다(실측: 막차 trip 만 반환).
// 그래서 앱의 "배차 약 N분" 표시는 이 표에서 나온다. 지하철은 OTP 의 앞뒤 열차 시각을 쓴다.
package headway

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Load 는 route_id → 배차간격(초)을 돌려준다. 한 노선에 여러 행이면 가장 짧은 값(가장 잦은 배차)을 쓴다.
func Load(zipPath string) (map[string]int, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	trips, err := readCSV(zr, "trips.txt")
	if err != nil {
		return nil, err
	}
	tripRoute := map[string]string{}
	for _, r := range trips {
		tripRoute[r["trip_id"]] = r["route_id"]
	}
	freqs, err := readCSV(zr, "frequencies.txt")
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, r := range freqs {
		route, ok := tripRoute[r["trip_id"]]
		if !ok {
			continue
		}
		h, err := strconv.Atoi(strings.TrimSpace(r["headway_secs"]))
		if err != nil || h <= 0 {
			continue
		}
		if cur, ok := out[route]; !ok || h < cur {
			out[route] = h
		}
	}
	return out, nil
}

func readCSV(zr *zip.ReadCloser, name string) ([]map[string]string, error) {
	for _, f := range zr.File {
		if f.Name != name {
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
			row := make(map[string]string, len(header))
			for i, h := range header {
				if i < len(rec) {
					row[h] = rec[i]
				}
			}
			rows = append(rows, row)
		}
		return rows, nil
	}
	return nil, fmt.Errorf("%s 없음", name)
}
