// Package ktdb 는 국가교통DB GTFS 파일럿(2025-03 평일 1일)에서 도시철도만 읽는다.
// 파일럿 route_type 은 비표준(1 = 도시철도/경전철)이고 stop_id 는 RS_, route_id 는 RR_ 접두사라 버스 ID 와 겹치지 않는다.
package ktdb

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Row = map[string]string

const utf8BOM = "\xef\xbb\xbf"

// Subway 는 bbox 를 지나는 도시철도 trip 과 그 참조 행이다. 각 슬라이스는 원본 열 순서 그대로 담긴다.
type Subway struct {
	Header    map[string][]string // 파일명 → 헤더
	Routes    []Row
	Trips     []Row
	StopTimes []Row
	Stops     []Row
	Transfers []Row
	Calendar  []Row
}

type BBox struct{ MinLon, MinLat, MaxLon, MaxLat float64 }

func (b BBox) Contains(lon, lat float64) bool {
	return lon >= b.MinLon && lon <= b.MaxLon && lat >= b.MinLat && lat <= b.MaxLat
}

// Load 는 dir 의 파일럿 txt 를 읽어 route_type=="1"(도시철도) 이면서 bbox 안 정류장을 하나라도 지나는 trip 만 남긴다.
func Load(dir string, bbox BBox) (*Subway, error) {
	s := &Subway{Header: map[string][]string{}}

	routes, err := s.read(dir, "routes.txt")
	if err != nil {
		return nil, err
	}
	keepRoutes := map[string]bool{}
	for _, r := range routes {
		if r["route_type"] == "1" {
			keepRoutes[r["route_id"]] = true
			s.Routes = append(s.Routes, r)
		}
	}

	stops, err := s.read(dir, "stops.txt")
	if err != nil {
		return nil, err
	}
	stopByID := map[string]Row{}
	inBox := map[string]bool{}
	for _, r := range stops {
		stopByID[r["stop_id"]] = r
		lon, e1 := strconv.ParseFloat(r["stop_lon"], 64)
		lat, e2 := strconv.ParseFloat(r["stop_lat"], 64)
		if e1 == nil && e2 == nil && bbox.Contains(lon, lat) {
			inBox[r["stop_id"]] = true
		}
	}

	trips, err := s.read(dir, "trips.txt")
	if err != nil {
		return nil, err
	}
	tripRoute := map[string]string{}
	for _, t := range trips {
		if keepRoutes[t["route_id"]] {
			tripRoute[t["trip_id"]] = t["route_id"]
		}
	}

	// stop_times 는 1.4GB 라 두 번 스트리밍한다: 1) bbox 를 지나는 trip 확정 2) 그 trip 의 행 수집.
	keepTrips := map[string]bool{}
	err = s.stream(dir, "stop_times.txt", func(r Row) {
		if _, ok := tripRoute[r["trip_id"]]; ok && inBox[r["stop_id"]] {
			keepTrips[r["trip_id"]] = true
		}
	})
	if err != nil {
		return nil, err
	}
	usedStops := map[string]bool{}
	err = s.stream(dir, "stop_times.txt", func(r Row) {
		if keepTrips[r["trip_id"]] {
			s.StopTimes = append(s.StopTimes, r)
			usedStops[r["stop_id"]] = true
		}
	})
	if err != nil {
		return nil, err
	}
	for _, t := range trips {
		if keepTrips[t["trip_id"]] {
			s.Trips = append(s.Trips, t)
		}
	}
	usedRoutes := map[string]bool{}
	for _, t := range s.Trips {
		usedRoutes[t["route_id"]] = true
	}
	s.Routes = filter(s.Routes, func(r Row) bool { return usedRoutes[r["route_id"]] })
	for id := range usedStops {
		if r, ok := stopByID[id]; ok {
			s.Stops = append(s.Stops, r)
		}
	}
	transfers, err := s.read(dir, "transfers.txt")
	if err != nil {
		return nil, err
	}
	s.Transfers = filter(transfers, func(r Row) bool { return usedStops[r["from_stop_id"]] && usedStops[r["to_stop_id"]] })
	s.Calendar, err = s.read(dir, "calendar.txt")
	if err != nil {
		return nil, err
	}
	if len(s.Trips) == 0 {
		return nil, fmt.Errorf("ktdb: bbox 안 도시철도 trip 이 없다 (%s)", dir)
	}
	return s, nil
}

func (s *Subway) read(dir, name string) ([]Row, error) {
	var rows []Row
	err := s.stream(dir, name, func(r Row) { rows = append(rows, r) })
	return rows, err
}

func (s *Subway) stream(dir, name string, fn func(Row)) error {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	rd := csv.NewReader(f)
	rd.ReuseRecord = true
	first, err := rd.Read()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	// ReuseRecord 라 Read 가 돌려준 슬라이스는 다음 Read 에서 덮인다 → 헤더는 복사본을 쓴다.
	hdr := append([]string(nil), first...)
	hdr[0] = strings.TrimPrefix(hdr[0], utf8BOM) // 파일럿 일부 파일은 UTF-8 BOM 으로 시작한다
	s.Header[name] = hdr
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		row := make(Row, len(hdr))
		for i, h := range hdr {
			if i < len(rec) {
				row[h] = rec[i]
			}
		}
		fn(row)
	}
}

func filter(rows []Row, keep func(Row) bool) []Row {
	out := rows[:0:0]
	for _, r := range rows {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}
