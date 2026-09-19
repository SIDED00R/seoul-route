package osm

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// RailNode 는 선로 위의 OSM 노드 하나. ID 가 같으면 같은 지점이다(선로끼리 이어지는 곳).
type RailNode struct {
	ID       int64
	Lat, Lon float64
}

// RailWay 는 노선 관계(type=route)에 속한 선로 하나. Ref 는 그 관계의 ref("2", "경의·중앙", "Silim").
type RailWay struct {
	Ref   string
	ID    int64
	Nodes []RailNode
}

// LoadRailWays 는 otp/extract_rail.py 가 만든 CSV(ref, way_id, seq, node_id, lat, lon 헤더, ref·way_id·seq 순 정렬)를 읽는다.
// 파일이 없으면 os.ErrNotExist 를 감싼 오류를 돌려준다.
func LoadRailWays(path string) ([]RailWay, error) {
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
	for _, k := range []string{"ref", "way_id", "node_id", "lat", "lon"} {
		if _, ok := col[k]; !ok {
			return nil, fmt.Errorf("%s: %s 열 없음", path, k)
		}
	}
	var out []RailWay
	for line := 2; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		wayID, e1 := strconv.ParseInt(rec[col["way_id"]], 10, 64)
		nodeID, e2 := strconv.ParseInt(rec[col["node_id"]], 10, 64)
		lat, e3 := strconv.ParseFloat(rec[col["lat"]], 64)
		lon, e4 := strconv.ParseFloat(rec[col["lon"]], 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			return nil, fmt.Errorf("%s:%d: 숫자 파싱 실패", path, line)
		}
		ref := strings.TrimSpace(rec[col["ref"]])
		if n := len(out); n == 0 || out[n-1].Ref != ref || out[n-1].ID != wayID {
			out = append(out, RailWay{Ref: ref, ID: wayID})
		}
		w := &out[len(out)-1]
		w.Nodes = append(w.Nodes, RailNode{ID: nodeID, Lat: lat, Lon: lon})
	}
	return out, nil
}
