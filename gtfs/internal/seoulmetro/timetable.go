// Package seoulmetro 는 공공데이터포털 "서울교통공사_서울 도시철도 열차운행시각표"(1~9호선, 요일별, 열차코드 포함)를
// 읽어 열차 단위로 묶는다. 파일은 otp/fetch_metro_timetable.py 가 내려받아 UTF-8 로 저장한다.
package seoulmetro

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// StopTime 은 한 열차의 한 정차. Arr 은 기점에서, Dep 은 종점에서 빈 문자열. 시각은 "HH:MM:SS", 자정 넘김은 "24:xx:xx".
type StopTime struct {
	Code string // 역사코드(4자리, 국가교통DB 파일럿 stop_id 뒷자리와 같다)
	Name string
	Arr  string
	Dep  string
}

// Train 은 (호선, 요일, 열차코드) 하나. Stops 는 시각순.
type Train struct {
	Line    string // "1".."9"
	Day     string // DAY(평일) / SAT(토) / END(일·공휴일)
	Code    string // 열차코드
	Dir     string // UP / DOWN / IN(2호선 내선) / OUT(외선)
	Express bool
	Origin  string
	Dest    string
	Stops   []StopTime
}

type Timetable struct {
	Trains   []Train // (Line, Day, Code) 순으로 정렬
	NRows    int
	NPassing int // 급행 통과역 행(한쪽 시각만 "00:00:00")으로 뺀 정차
	NNoTime  int // 도착·출발이 모두 결측("00:00:00" 포함)이라 뺀 정차(현 데이터 0)
}

func blankZero(s string) string {
	if s == "00:00:00" {
		return ""
	}
	return s
}

var cols = []string{"호선", "역사코드", "역사명", "주중주말", "방향", "급행여부", "열차코드", "열차도착시간", "열차출발시간",
	"출발역", "도착역"}

// Load 는 CSV를 열차별로 묶고 정차 시각순으로 정렬한다.
func Load(path string) (*Timetable, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	if b, _ := br.Peek(3); bytes.Equal(b, []byte("\xef\xbb\xbf")) { // BOM 뒤에 따옴표 필드가 오면 csv 가 파싱 오류를 낸다
		br.Discard(3)
	}
	r := csv.NewReader(br)
	r.ReuseRecord = true
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: 헤더 없음: %w", path, err)
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.TrimSpace(h)] = i
	}
	for _, c := range cols {
		if _, ok := idx[c]; !ok {
			return nil, fmt.Errorf("%s: %s 열 없음", path, c)
		}
	}
	type key struct{ line, day, code string }
	trains := map[key]*Train{}
	out := &Timetable{}
	n := 0
	for line := 2; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		n++
		k := key{rec[idx["호선"]], rec[idx["주중주말"]], rec[idx["열차코드"]]}
		t, ok := trains[k]
		if !ok {
			t = &Train{Line: k.line, Day: k.day, Code: k.code, Dir: rec[idx["방향"]], Express: rec[idx["급행여부"]] == "1",
				Origin: rec[idx["출발역"]], Dest: rec[idx["도착역"]]}
			trains[k] = t
		}
		// 급행에서 한쪽 시각만 00:00:00인 행은 통과역이다. 실제 자정은 24:00:00으로 기록된다.
		arrRaw, depRaw := rec[idx["열차도착시간"]], rec[idx["열차출발시간"]]
		if t.Express && (arrRaw == "00:00:00") != (depRaw == "00:00:00") {
			out.NPassing++
			continue
		}
		st := StopTime{Code: rec[idx["역사코드"]], Name: rec[idx["역사명"]], Arr: blankZero(arrRaw), Dep: blankZero(depRaw)}
		if st.Arr == "" && st.Dep == "" {
			out.NNoTime++
			continue
		}
		t.Stops = append(t.Stops, st)
	}
	out.NRows = n
	for _, t := range trains {
		sort.SliceStable(t.Stops, func(i, j int) bool { return timeKey(t.Stops[i]) < timeKey(t.Stops[j]) })
		out.Trains = append(out.Trains, *t)
	}
	sort.Slice(out.Trains, func(i, j int) bool {
		a, b := out.Trains[i], out.Trains[j]
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		return a.Code < b.Code
	})
	return out, nil
}

// timeKey: 출발시각, 종점(출발 없음)은 도착시각. "24:05:00" 이 "23:59:00" 뒤에 오도록 문자열 비교로 충분하다.
func timeKey(s StopTime) string {
	if s.Dep != "" {
		return s.Dep
	}
	return s.Arr
}
