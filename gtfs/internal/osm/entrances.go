// Package osm 은 otp/extract_entrances.py 가 OSM 에서 뽑은 지하철 출입구 CSV 를 읽는다.
package osm

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Entrance 는 OSM railway=subway_entrance 노드 하나. Ref 는 출구 번호("3", "5-1"), 없으면 빈 문자열.
type Entrance struct {
	Lat, Lon float64
	Ref      string
	Name     string
}

// LoadEntrances 는 CSV(lat, lon, ref, name 헤더)를 읽는다. 파일이 없으면 os.ErrNotExist 를 감싼 오류를 돌려준다.
func LoadEntrances(path string) ([]Entrance, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: 헤더 없음: %w", path, err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimPrefix(strings.TrimSpace(h), "\xef\xbb\xbf")] = i // UTF-8 BOM
	}
	for _, k := range []string{"lat", "lon", "ref", "name"} {
		if _, ok := col[k]; !ok {
			return nil, fmt.Errorf("%s: %s 열 없음", path, k)
		}
	}
	var out []Entrance
	for line := 2; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		lat, err1 := strconv.ParseFloat(rec[col["lat"]], 64)
		lon, err2 := strconv.ParseFloat(rec[col["lon"]], 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("%s:%d: 좌표 파싱 실패", path, line)
		}
		out = append(out, Entrance{Lat: lat, Lon: lon, Ref: strings.TrimSpace(rec[col["ref"]]),
			Name: strings.TrimSpace(rec[col["name"]])})
	}
	return out, nil
}
