// Package kric 는 레일포털(KRIC) Open API 로 받은 코레일·민자 노선의 역별 시각표(otp/fetch_kric_timetable.py 산출)를
// 읽어 열차 단위로 묶는다. 파일 두 개: kric-stations.csv(노선별 역 순서·좌표), kric-timetable.csv(역별·요일별 정차).
package kric

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
)

type Station struct {
	Code  string // 레일포털 역 코드(예 K114). 파일럿 stop_id 와는 무관하다
	Name  string
	Order int // 노선 안 역 순서(stinConsOrdr)
	Lat   float64
	Lon   float64
}

// Line 은 파일럿 route_id 의 노선 코드(RR_ACC1_S-1-<Pilot>-…) 하나에 대응한다.
type Line struct {
	Pilot    string // KJ, SD, …
	Opr      string // 운영기관 코드(KR 코레일, AR 공항철도, DX 신분당 …)
	Code     string // 레일포털 선 코드(K4 …)
	Name     string // 레일포털 노선명(경의중앙 …)
	Stations []Station
	HasSat   bool // 토요일(dayCd 7) 시각표가 있는지. 없으면 휴일 시각표를 토·일에 쓴다
}

// StopTime 의 시각은 "HH:MM:SS". 자정 이후(00~02시대)는 같은 운행일의 심야라 24 를 더해 "24:xx:xx" 로 둔다.
type StopTime struct {
	Code string
	Arr  string // 기점은 빈 문자열
	Dep  string // 종점은 빈 문자열
}

type Train struct {
	Line  string // Pilot 코드
	Day   string // 8 평일 / 7 토 / 9 휴일
	No    string
	Org   string // 기점 역 코드
	Tmn   string // 종점 역 코드
	Stops []StopTime
}

type Timetable struct {
	Lines  map[string]*Line // Pilot 코드 → 노선
	Trains []Train          // (Line, Day, No) 순
	NRows  int
	NDup   int // 같은 열차가 같은 역에 두 번 있는 행(뒤 것을 버림)
}

// LateNightHour 미만의 시각은 전날 운행의 심야로 본다.
const LateNightHour = 3

// Load 는 두 CSV 를 읽는다. 역 파일이 있어야 노선이 생기고, 그 노선의 시각표 행만 받는다.
func Load(stationsPath, timetablePath string) (*Timetable, error) {
	tt := &Timetable{Lines: map[string]*Line{}}
	srows, err := readCSV(stationsPath,
		[]string{"line", "opr", "ln_cd", "ln_name", "stin_cd", "order", "name", "lat", "lon"})
	if err != nil {
		return nil, err
	}
	for _, r := range srows {
		l := tt.Lines[r["line"]]
		if l == nil {
			l = &Line{Pilot: r["line"], Opr: r["opr"], Code: r["ln_cd"], Name: r["ln_name"]}
			tt.Lines[r["line"]] = l
		}
		order, _ := strconv.Atoi(r["order"])
		lat, _ := strconv.ParseFloat(r["lat"], 64)
		lon, _ := strconv.ParseFloat(r["lon"], 64)
		l.Stations = append(l.Stations, Station{Code: r["stin_cd"], Name: r["name"], Order: order, Lat: lat, Lon: lon})
	}
	for _, l := range tt.Lines {
		sort.Slice(l.Stations, func(i, j int) bool { return l.Stations[i].Order < l.Stations[j].Order })
	}
	trows, err := readCSV(timetablePath, []string{"line", "day", "trn_no", "stin_cd", "arv", "dep", "org", "tmn"})
	if err != nil {
		return nil, err
	}
	type key struct{ line, day, no string }
	trains := map[key]*Train{}
	seen := map[key]map[string]bool{}
	var order []key
	for _, r := range trows {
		l := tt.Lines[r["line"]]
		if l == nil {
			continue
		}
		tt.NRows++
		if r["day"] == "7" {
			l.HasSat = true
		}
		k := key{r["line"], r["day"], r["trn_no"]}
		t := trains[k]
		if t == nil {
			t = &Train{Line: k.line, Day: k.day, No: k.no, Org: r["org"], Tmn: r["tmn"]}
			trains[k] = t
			seen[k] = map[string]bool{}
			order = append(order, k)
		}
		if seen[k][r["stin_cd"]] {
			tt.NDup++
			continue
		}
		seen[k][r["stin_cd"]] = true
		t.Stops = append(t.Stops, StopTime{Code: r["stin_cd"], Arr: hhmmss(r["arv"]), Dep: hhmmss(r["dep"])})
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.line != b.line {
			return a.line < b.line
		}
		if a.day != b.day {
			return a.day < b.day
		}
		return a.no < b.no
	})
	for _, k := range order {
		t := trains[k]
		sort.SliceStable(t.Stops, func(i, j int) bool { return timeKey(t.Stops[i]) < timeKey(t.Stops[j]) })
		tt.Trains = append(tt.Trains, *t)
	}
	return tt, nil
}

// hhmmss: "050100" → "05:01:00", "" → "", "001730"(자정 이후) → "24:17:30".
func hhmmss(s string) string {
	if len(s) != 6 {
		return ""
	}
	h, err := strconv.Atoi(s[:2])
	if err != nil {
		return ""
	}
	if h < LateNightHour {
		h += 24
	}
	return fmt.Sprintf("%02d:%s:%s", h, s[2:4], s[4:6])
}

// timeKey: 출발시각, 종점(출발 없음)은 도착시각. 문자열 비교로 충분하다("24:xx" 가 "23:xx" 뒤).
func timeKey(s StopTime) string {
	if s.Dep != "" {
		return s.Dep
	}
	return s.Arr
}

func readCSV(path string, cols []string) ([]map[string]string, error) {
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
	idx := map[string]int{}
	for i, h := range header {
		idx[h] = i
	}
	for _, c := range cols {
		if _, ok := idx[c]; !ok {
			return nil, fmt.Errorf("%s: %s 열 없음", path, c)
		}
	}
	var out []map[string]string
	for line := 2; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		row := map[string]string{}
		for _, c := range cols {
			row[c] = rec[idx[c]]
		}
		out = append(out, row)
	}
	return out, nil
}
