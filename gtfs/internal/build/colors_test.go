package build

import (
	"archive/zip"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
)

// 시각표 CSV 가 없으면 1~9호선과 레일포털 노선도 파일럿 행으로 나온다. 그때도 노선 코드로 색을 찾아야 한다.
// 파일럿 코드는 0 을 채운 "01"~"09" 라 호선 표 키("1"~"9")와 다르다.
func TestPilotSubwayColors(t *testing.T) {
	sub := &ktdb.Subway{
		Routes: []ktdb.Row{
			{"route_id": "RR_ACC1_S-1-01-1D", "route_short_name": "수도권1호선", "route_long_name": "1호선<하행>"},
			{"route_id": "RR_ACC1_S-1-09-1D", "route_short_name": "서울9호선", "route_long_name": "9호선<하행>"},
			{"route_id": "RR_ACC1_S-1-KJ-1D", "route_short_name": "경의중앙선", "route_long_name": "경의중앙선<하행>"},
			{"route_id": "RR_ACC1_S-1-SH-1D", "route_short_name": "서해선", "route_long_name": "서해선<하행>"},
		},
	}
	out := filepath.Join(t.TempDir(), "g.zip")
	if _, err := Build(out, []BusRoute{sampleRoute()}, sub, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	got := map[string]string{}
	for _, r := range readTable(t, zr, "routes.txt")[1:] {
		got[r[0]] = r[5] + "/" + r[6]
	}
	want := map[string]string{
		"RR_ACC1_S-1-01-1D": "0052A4/FFFFFF",
		"RR_ACC1_S-1-09-1D": "BDB092/000000", // 밝은 배경은 검은 글자
		"RR_ACC1_S-1-KJ-1D": "77C4A3/000000",
		"RR_ACC1_S-1-SH-1D": "81A914/000000", // 코드 표에 없어 이름으로 찾는다
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s 색=%q, want %q", id, got[id], w)
		}
	}
}

// 버스는 유형별 색이고, 서울시 색 체계 밖 유형(공항·인천·경기·관광)은 비워 앱이 기본 팔레트를 쓰게 한다.
func TestBusColors(t *testing.T) {
	cases := map[string]string{"3": "0068B7/FFFFFF", "4": "53B332/FFFFFF", "2": "53B332/FFFFFF", "6": "E60012/FFFFFF",
		"5": "F2B70A/000000", "15": "3D5BAB/FFC600", "1": "/", "7": "/", "8": "/", "10": "/"}
	for typ, want := range cases {
		c := busColor(typ)
		if got := c + "/" + textColor(c); got != want {
			t.Errorf("routeType %s 색=%q, want %q", typ, got, want)
		}
	}
	if strings.Contains(textColor(""), "F") { // 색이 없으면 글자색도 없다
		t.Errorf("빈 색의 글자색=%q", textColor(""))
	}
}
