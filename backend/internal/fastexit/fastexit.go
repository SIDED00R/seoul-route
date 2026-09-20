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
	ElvtrNo  string `json:"elvtrNo"`          // 승강기번호. 에스컬레이터는 이 번호로 운행방향을 찾는다
	FacPstn  string `json:"facPstnNm"`        // 설비 위치 설명. "환승통로(…)" 로 시작하면 환승 통로로 이어진다
}

// Facility 는 한 설비 종류와 그 앞 칸-문 목록(앞 칸부터).
type Facility struct {
	Name  string   `json:"facility"`
	Doors []string `json:"doors"`
}

// Index 는 (노선, 역) → 행. escalators 는 승강기번호 → 에스컬레이터(운행방향). 비어 있으면 에스컬레이터를 싣지 않는다.
type Index struct {
	rows       map[[2]string][]Row
	escalators map[string]Escalator
}

// 역 이름이 바뀌었는데 생성 GTFS(국가교통DB 2025-03 기준 역 이름)에는 옛 이름으로 남은 역.
// GTFS 역 이름 출처를 바꾸면 다시 대조한다.
var renamed = map[string]string{"불암산": "당고개", "자양": "뚝섬유원지", "이수": "총신대입구"}

// facilityOrder 는 응답에 싣는 설비와 표시 순서다.
// "환승통로 …" 는 그 설비가 환승 통로로 이어질 때 쓰는 이름이다.
var facilityOrder = []string{"에스컬레이터", "계단", "환승통로 에스컬레이터", "환승통로 계단"}

// Load 는 cmd/fastexit 이 저장한 두 JSON(빠른하차 행 배열, 에스컬레이터 행 배열)을 읽는다. 에스컬레이터 파일이
// 없으면 계단만 싣는다.
func Load(path, escalatorPath string) (*Index, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []Row
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	esc, err := LoadEscalators(escalatorPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return NewIndex(rows, esc), nil
}

// NewIndex 는 빠른하차 행과 에스컬레이터 운행방향표로 색인을 만든다. escalators 가 비면 에스컬레이터는 싣지 않는다
// — 올라가는지 내려가는지 모르는 채로 보여 주면 내릴 때 쓸 수 없는 칸을 알려 주게 된다.
func NewIndex(rows []Row, escalators map[string]Escalator) *Index {
	ix := &Index{rows: map[[2]string][]Row{}, escalators: escalators}
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
		name, ok := ix.facilityName(r)
		if !ok {
			continue
		}
		if !contains(doors[name], d) {
			doors[name] = append(doors[name], d)
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

// facilityName 은 이 행을 어떤 이름으로 실을지. 에스컬레이터는 올라가는 것만 싣고(내릴 때 쓰는 안내다), 운행방향을
// 모르면 싣지 않는다. 환승 통로로 이어지는 설비는 "환승통로 …" 로 따로 묶는다.
func (ix *Index) facilityName(r Row) (string, bool) {
	name := strings.TrimSpace(r.Facility)
	transfer := strings.HasPrefix(strings.TrimSpace(r.FacPstn), "환승통로")
	if name == "에스컬레이터" {
		e, ok := ix.escalators[strings.TrimSpace(r.ElvtrNo)]
		if !ok || !e.goesUp() {
			return "", false
		}
		transfer = transfer || e.transfer()
	}
	if transfer {
		name = "환승통로 " + name
	}
	return name, true
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
