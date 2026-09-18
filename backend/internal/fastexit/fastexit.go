// Package fastexit 은 지하철 하차역에서 계단·에스컬레이터·엘리베이터가 어느 칸-문 앞에 있는지 찾는다.
// 자료는 공공데이터포털 「서울교통공사_빠른하차정보」(1~8호선)를 cmd/fastexit 이 받아 둔 JSON 파일이다.
// 이 자료에는 설비가 어느 출구·환승 통로로 이어지는지가 없어 설비 종류와 칸-문 번호만 준다.
package fastexit

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Row 는 API 의 한 행: 한 역·한 방향의 설비 하나.
type Row struct {
	Line     string `json:"lineNm"`           // "2호선"
	Station  string `json:"stnNm"`            // "사당", "서울역"
	Side     string `json:"upbdnbSe"`         // 상행·하행
	Toward   string `json:"drtnInfo"`         // 그 방향 열차가 향하는 다음 역(방면)
	Door     string `json:"qckgffVhclDoorNo"` // 칸-문 "3-3". 값이 없으면 "NA-NA", 두 문 사이면 "3-2,3-3 사이"
	Facility string `json:"plfmCmgFac"`       // 계단·에스컬레이터·엘리베이터
}

// Facility 는 한 설비 종류와 그 앞 칸-문 목록(앞 칸부터).
type Facility struct {
	Name  string   `json:"facility"`
	Doors []string `json:"doors"`
}

// Index 는 (노선, 역) → 행.
type Index struct {
	rows map[[2]string][]Row
}

// 역 이름이 바뀌었는데 생성 GTFS(국가교통DB 2025-03 기준 역 이름)에는 옛 이름으로 남은 역. 2026-09-19 API 전체
// 276개 역을 GTFS 1~8호선 역과 대조해 이름이 안 맞은 것이 이 셋뿐이었다. GTFS 역 이름 출처를 바꾸면 다시 대조한다.
var renamed = map[string]string{"불암산": "당고개", "자양": "뚝섬유원지", "이수": "총신대입구"}

// facilityOrder 는 응답에 싣는 설비와 그 순서. 엘리베이터는 넣지 않는다(사용자 결정 2026-09-19).
var facilityOrder = []string{"에스컬레이터", "계단"}

// Load 는 cmd/fastexit 이 저장한 JSON(행 배열)을 읽는다.
func Load(path string) (*Index, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []Row
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	return NewIndex(rows), nil
}

func NewIndex(rows []Row) *Index {
	ix := &Index{rows: map[[2]string][]Row{}}
	for _, r := range rows {
		k := [2]string{strings.TrimSpace(r.Line), canon(r.Station)}
		ix.rows[k] = append(ix.rows[k], r)
	}
	return ix
}

// Len 은 자료가 있는 (노선, 역) 수.
func (ix *Index) Len() int { return len(ix.rows) }

// Lookup 은 line("4호선") 열차로 prevStop 을 지나 station 에 내릴 때의 설비별 칸-문을 돌려준다. 승강장 방향은
// prevStop 쪽으로 가는 방향의 반대편으로 정한다. 역이 자료에 없거나 방향을 정할 수 없으면 nil(틀린 칸을 주지 않는다).
func (ix *Index) Lookup(line, station, prevStop string) []Facility {
	rows := ix.rows[[2]string{strings.TrimSpace(line), canon(station)}]
	prev := canon(prevStop)
	back := "" // prevStop 으로 가는 방향(상행·하행)
	for _, r := range rows {
		if prev != "" && canon(r.Toward) == prev {
			back = r.Side
			break
		}
	}
	if back == "" {
		return nil
	}
	doors := map[string][]string{}
	for _, r := range rows {
		d := strings.TrimSpace(r.Door)
		if r.Side == back || d == "" || strings.HasPrefix(d, "NA") {
			continue
		}
		if !contains(doors[r.Facility], d) {
			doors[r.Facility] = append(doors[r.Facility], d)
		}
	}
	var out []Facility
	for _, name := range facilityOrder {
		if ds := doors[name]; len(ds) > 0 {
			sort.SliceStable(ds, func(i, j int) bool { return doorKey(ds[i]) < doorKey(ds[j]) })
			out = append(out, Facility{Name: name, Doors: ds})
		}
	}
	return out
}

// canon 은 역 이름을 비교용으로 맞춘다: 괄호 뒤("사당(4호선)")와 끝의 "역"("서울역")을 떼고, 바뀐 이름은 옛 이름으로.
func canon(name string) string {
	if i := strings.Index(name, "("); i >= 0 {
		name = name[:i]
	}
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > 2 && r[len(r)-1] == '역' {
		name = string(r[:len(r)-1])
	}
	if old, ok := renamed[name]; ok {
		return old
	}
	return name
}

// doorKey 는 "3-2" → 32. 첫 칸-문만 본다("3-2,3-3 사이" → 32).
func doorKey(d string) int {
	car, door, _ := strings.Cut(d, "-")
	c, _ := strconv.Atoi(strings.TrimSpace(car))
	n := 0
	for _, ch := range door {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return c*10 + n
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
