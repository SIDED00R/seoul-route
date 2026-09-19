package fastexit

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// 에스컬레이터 행에는 승강기번호를 붙인다 — 그 번호로 운행방향을 찾는다.
func row(line, stn, side, toward, door, fac string) Row {
	r := Row{Line: line, Station: stn, Side: side, Toward: toward, Door: door, Facility: fac}
	if fac == "에스컬레이터" {
		r.ElvtrNo = "UP-" + stn + door // sample() 의 방향표에서 전부 상행으로 둔다
	}
	return r
}

// upEscalators 는 sample() 의 에스컬레이터를 전부 상행으로 두는 방향표.
func upEscalators(rows []Row) map[string]Escalator {
	out := map[string]Escalator{}
	for _, r := range rows {
		if r.ElvtrNo != "" {
			out[r.ElvtrNo] = Escalator{No: r.ElvtrNo, Direction: "상행", Section: "B2-B1", Position: "1번 출입구"}
		}
	}
	return out
}

// 사당 2호선: 상행은 방배 방면, 하행은 낙성대 방면.
func sample() *Index {
	rows := []Row{
		row("2호선", "사당", "상행", "방배", "8-1", "계단"),
		row("2호선", "사당", "상행", "방배", "3-3", "계단"),
		row("2호선", "사당", "상행", "방배", "3-3", "에스컬레이터"),
		row("2호선", "사당", "상행", "방배", "3-3", "에스컬레이터"), // 같은 칸의 설비가 둘이면 행도 둘이다
		row("2호선", "사당", "상행", "방배", "10-4", "엘리베이터"),
		row("2호선", "사당", "상행", "방배", "NA-NA", "엘리베이터"),
		row("2호선", "사당", "하행", "낙성대", "5-2", "계단"),
		row("4호선", "사당", "상행", "총신대입구(이수)", "1-1", "계단"),
		row("1호선", "서울역", "하행", "남영", "2-3", "에스컬레이터"),
		row("1호선", "서울역", "상행", "시청", "9-3", "에스컬레이터"),
		row("7호선", "건대입구", "상행", "자양", "4-2", "계단"),
		row("7호선", "건대입구", "하행", "어린이대공원", "6-1", "계단"),
		row("3호선", "종로3가", "상행", "안국", "3-2,3-3 사이", "엘리베이터"),
		row("3호선", "종로3가", "상행", "안국", "1-1", "엘리베이터"),
		row("3호선", "종로3가", "하행", "을지로3가", "7-1", "엘리베이터"),
		row("3호선", "충무로", "상행", "을지로3가", "10-2", "계단"),
		row("3호선", "충무로", "상행", "을지로3가", "3-2,3-3 사이", "계단"),
		row("3호선", "충무로", "하행", "동대입구", "5-2", "계단"),
	}
	return NewIndex(rows, upEscalators(rows))
}

func TestLookupDirectionAndOrder(t *testing.T) {
	ix := sample()
	// 낙성대를 지나 사당에 내리면 방배 방면(상행) 승강장이다. 설비 순서는 에스컬레이터·계단·엘리베이터, 칸은 앞에서부터.
	got := ix.Lookup("2호선", "사당(2호선)", "낙성대")
	want := []Facility{{"에스컬레이터", []string{"3-3"}}, {"계단", []string{"3-3", "8-1"}}} // 엘리베이터는 안 싣는다
	if !reflect.DeepEqual(got, want) {
		t.Errorf("상행: %+v", got)
	}
	if got := ix.Lookup("2호선", "사당", "방배"); !reflect.DeepEqual(got, []Facility{{"계단", []string{"5-2"}}}) {
		t.Errorf("하행: %+v", got)
	}
	// 엘리베이터만 있는 역은 비운다.
	if got := ix.Lookup("3호선", "종로3가", "을지로3가"); got != nil {
		t.Errorf("엘리베이터만: %+v", got)
	}
	// 두 문 사이 표기는 그대로 싣고 첫 칸-문으로 줄 세운다.
	got = ix.Lookup("3호선", "충무로", "동대입구")
	if !reflect.DeepEqual(got, []Facility{{"계단", []string{"3-2,3-3 사이", "10-2"}}}) {
		t.Errorf("사이 표기: %+v", got)
	}
}

func TestLookupNames(t *testing.T) {
	ix := sample()
	// API 는 "서울역", GTFS 는 "서울(1호선)".
	if got := ix.Lookup("1호선", "서울(1호선)", "시청(1호선)"); len(got) != 1 || got[0].Doors[0] != "2-3" {
		t.Errorf("서울역: %+v", got)
	}
	// API 는 새 이름 "자양", GTFS 는 옛 이름 "뚝섬유원지".
	if got := ix.Lookup("7호선", "건대입구(7호선)", "뚝섬유원지"); len(got) != 1 || got[0].Doors[0] != "6-1" {
		t.Errorf("바뀐 역 이름: %+v", got)
	}
}

// 방향을 정할 수 없으면 비운다.
func TestLookupUnknown(t *testing.T) {
	ix := sample()
	for _, c := range [][3]string{
		{"2호선", "사당", "교대"},  // 직전 정차역이 어느 방면과도 맞지 않는다
		{"2호선", "사당", ""},    // 직전 정차역을 모른다
		{"9호선", "사당", "낙성대"}, // 자료에 없는 노선
		{"2호선", "강남", "역삼"},  // 자료에 없는 역
		{"4호선", "사당", "남태령"}, // 반대 방향 행이 없는 역(한쪽 방면만 있음)
	} {
		if got := ix.Lookup(c[0], c[1], c[2]); got != nil {
			t.Errorf("%v: %+v", c, got)
		}
	}
}

func TestLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.json")
	os.WriteFile(p, []byte(`[{"lineNm":"2호선","stnNm":"사당","upbdnbSe":"상행","drtnInfo":"방배",`+
		`"qckgffVhclDoorNo":"3-3","plfmCmgFac":"계단","crtrYmd":"20241231"},`+
		`{"lineNm":"2호선","stnNm":"사당","upbdnbSe":"하행","drtnInfo":"낙성대","qckgffVhclDoorNo":"5-2","plfmCmgFac":"계단"}]`), 0o644)
	e := filepath.Join(t.TempDir(), "e.json")
	os.WriteFile(e, []byte(`[{"ESCAL_NO":"1806-600","DIRECTION":"상행","SECTION":"B2-B1","INSTL_PSTN":"1번 출입구"}]`), 0o644)
	ix, err := Load(p, e)
	if err != nil || ix.Len() != 1 {
		t.Fatalf("ix=%+v err=%v", ix, err)
	}
	if got := ix.Lookup("2호선", "사당", "낙성대"); len(got) != 1 || got[0].Doors[0] != "3-3" {
		t.Errorf("got=%+v", got)
	}
	// 에스컬레이터 파일이 없어도 계단은 나온다.
	if ix, err := Load(p, filepath.Join(t.TempDir(), "none.json")); err != nil || len(ix.Lookup("2호선", "사당", "낙성대")) != 1 {
		t.Errorf("방향표 없음: err=%v", err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "none.json"), e); err == nil {
		t.Error("없는 파일인데 오류가 없다")
	}
	os.WriteFile(p, []byte(`{"not":"array"}`), 0o644)
	if _, err := Load(p, e); err == nil {
		t.Error("형식이 다른데 오류가 없다")
	}
}

// 에스컬레이터는 올라가는 것만 싣는다. 방향을 모르면 싣지 않는다 — 내릴 때 쓸 수 없는 칸을 알려 주지 않는다.
func TestLookupEscalatorDirection(t *testing.T) {
	rows := []Row{
		{Line: "4호선", Station: "사당", Side: "하행", Toward: "남태령", Door: "1-1", Facility: "에스컬레이터", ElvtrNo: "DOWN"},
		{Line: "4호선", Station: "사당", Side: "하행", Toward: "남태령", Door: "3-3", Facility: "에스컬레이터", ElvtrNo: "UP"},
		{Line: "4호선", Station: "사당", Side: "하행", Toward: "남태령", Door: "7-2", Facility: "에스컬레이터", ElvtrNo: "없는번호"},
		{Line: "4호선", Station: "사당", Side: "하행", Toward: "남태령", Door: "5-2", Facility: "계단"},
		{Line: "4호선", Station: "사당", Side: "상행", Toward: "총신대입구", Door: "9-1", Facility: "계단"},
	}
	ix := NewIndex(rows, map[string]Escalator{
		"DOWN": {No: "DOWN", Direction: "하행", Section: "B2-B3", Position: "1번 출입구"},
		"UP":   {No: "UP", Direction: "상행", Section: "B3-B2", Position: "2번 출입구"},
	})
	got := ix.Lookup("4호선", "사당(4호선)", "총신대입구")
	want := []Facility{{"에스컬레이터", []string{"3-3"}}, {"계단", []string{"5-2"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("상행만 남아야 한다: %+v", got)
	}
	// 방향표가 아예 없으면 계단만.
	if got := NewIndex(rows, nil).Lookup("4호선", "사당", "총신대입구"); !reflect.DeepEqual(got,
		[]Facility{{"계단", []string{"5-2"}}}) {
		t.Errorf("방향표 없음: %+v", got)
	}
}

// 환승 통로로 이어지는 설비는 따로 묶는다. 설치위치(에스컬레이터)나 설비 위치 설명(계단)에서 알아낸다.
func TestLookupTransferPassage(t *testing.T) {
	rows := []Row{
		{Line: "3호선", Station: "종로3가", Side: "하행", Toward: "을지로3가", Door: "1-1", Facility: "에스컬레이터",
			ElvtrNo: "T"},
		{Line: "3호선", Station: "종로3가", Side: "하행", Toward: "을지로3가", Door: "4-2", Facility: "에스컬레이터",
			ElvtrNo: "P"},
		{Line: "3호선", Station: "종로3가", Side: "하행", Toward: "을지로3가", Door: "8-4", Facility: "계단",
			FacPstn: "환승통로(을지로3가 방면8-4)"},
		{Line: "3호선", Station: "종로3가", Side: "상행", Toward: "안국", Door: "2-2", Facility: "계단"},
	}
	ix := NewIndex(rows, map[string]Escalator{
		"T": {No: "T", Direction: "상행", Position: "환승통로(1,5호선)"},
		"P": {No: "P", Direction: "상행", Position: "3번 출입구"},
	})
	got := ix.Lookup("3호선", "종로3가", "안국")
	want := []Facility{{"에스컬레이터", []string{"4-2"}}, {"환승통로 에스컬레이터", []string{"1-1"}},
		{"환승통로 계단", []string{"8-4"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("환승통로 구분: %+v", got)
	}
}
