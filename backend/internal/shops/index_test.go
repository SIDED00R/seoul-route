package shops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func names(ms []Match) string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return strings.Join(out, " / ")
}

// 서울 위도에서 경도 0.001° ≈ 88m.
var sample = New([]Shop{
	{ID: "1", Name: "무교동북어국집", Lat: 37.5677, Lon: 126.9798},
	{ID: "2", Name: "원조무교동북어", Lat: 37.5000, Lon: 127.0000},
	{ID: "3", Name: "스타벅스 먼점", Lat: 37.5000, Lon: 127.0100},
	{ID: "4", Name: "스타벅스 앞점", Lat: 37.5000, Lon: 127.0010},
	{ID: "5", Name: "GS25 가나점", Lat: 37.5000, Lon: 127.0000},
	{ID: "6", Name: "동북보험대리점", Lat: 37.5677, Lon: 126.9790},
})

func TestSearch(t *testing.T) {
	cases := []struct {
		name, q string
		located bool
		want    string
	}{
		{"앞부분 일치가 먼저, 그다음 부분 일치", "무교동북", false, "무교동북어국집 / 원조무교동북어"},
		{"가까운 부분 일치보다 먼 앞부분 일치가 먼저", "무교동북", true, "무교동북어국집 / 원조무교동북어"},
		{"가운데 부분도 찾는다", "교동북어국", false, "무교동북어국집"},
		{"위치가 있으면 앞부분 일치 안에서 가까운 순", "스타벅", true, "스타벅스 앞점 / 스타벅스 먼점"},
		{"위치가 없으면 이름이 짧은 순(같으면 목록 순)", "스타벅", false, "스타벅스 먼점 / 스타벅스 앞점"},
		{"공백·대소문자 무시", "gs 25", false, "GS25 가나점"},
		{"한 글자는 찾지 않는다", "스", false, ""},
		{"없으면 빈 목록", "없는가게", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := names(sample.Search(c.q, 37.5, 127.0, c.located, 10)); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
	if got := sample.Search("스타벅", 37.5, 127.0, true, 1); len(got) != 1 || got[0].Name != "스타벅스 앞점" ||
		got[0].DistanceM < 80 || got[0].DistanceM > 95 {
		t.Errorf("limit·거리: %+v", got)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.csv")
	os.WriteFile(good, []byte("id,name,branch,category,address,lon,lat\n"+
		"A1,스타벅스,무교로점,카페,서울특별시 중구 무교로 15,126.9790000,37.5680000\n"+
		"A2,무교동북어국집,,백반/한정식,서울특별시 중구 을지로1길 38,126.9798518,37.5677434\n"), 0o644)
	x, err := Load(good)
	if err != nil {
		t.Fatal(err)
	}
	got := x.Search("북어", 0, 0, false, 10)
	if x.Len() != 2 || len(got) != 1 || got[0].Name != "무교동북어국집" || got[0].Category != "백반/한정식" ||
		got[0].Address != "서울특별시 중구 을지로1길 38" || got[0].Lat != 37.5677434 {
		t.Fatalf("len=%d got=%+v", x.Len(), got)
	}
	if got := x.Search("스타벅스무교", 0, 0, false, 10); len(got) != 1 || got[0].Name != "스타벅스 무교로점" {
		t.Errorf("지점명을 이은 이름: %+v", got)
	}

	bad := filepath.Join(dir, "bad.csv")
	os.WriteFile(bad, []byte("id,name,lat,lon\nA1,가,37.5,127\n"), 0o644)
	if _, err := Load(bad); err == nil || !strings.Contains(err.Error(), "헤더") {
		t.Errorf("헤더가 다르면 오류: %v", err)
	}
	coord := filepath.Join(dir, "coord.csv")
	os.WriteFile(coord, []byte("id,name,branch,category,address,lon,lat\nA1,가,,,,x,37.5\n"), 0o644)
	if _, err := Load(coord); err == nil || !strings.Contains(err.Error(), ":2:") {
		t.Errorf("좌표가 깨지면 줄 번호와 함께 오류: %v", err)
	}
	if _, err := Load(filepath.Join(dir, "none.csv")); err == nil {
		t.Error("파일이 없으면 오류")
	}
}
