package routestyle

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, routes string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gtfs.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create("routes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(routes)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// 색이 있는 노선만 담고, 색 없는 노선(파일럿 잔여·유형 밖 버스)은 빼 앱이 기본 팔레트를 쓰게 한다.
func TestLoad(t *testing.T) {
	path := writeZip(t, "route_id,agency_id,route_short_name,route_long_name,route_type,route_color,route_text_color\n"+
		"M_2,A_SEOULMETRO,2호선,2호선,1,00A84D,FFFFFF\n"+
		"K_WS,A_UI,우이신설,우이신설,1,B0CE18,000000\n"+
		"B_100100047,A_SEOULBUS,402,a ~ b,3,0068B7,FFFFFF\n"+
		"B_999,A_SEOULBUS,999,a ~ b,3,,\n")
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("노선 %d개: %v", len(got), got)
	}
	if got["M_2"] != (Style{Color: "00A84D", TextColor: "FFFFFF"}) {
		t.Errorf("M_2=%v", got["M_2"])
	}
	if got["K_WS"].TextColor != "000000" { // 밝은 배경은 검은 글자
		t.Errorf("K_WS=%v", got["K_WS"])
	}
	if _, ok := got["B_999"]; ok {
		t.Errorf("색 없는 노선이 들어갔다: %v", got["B_999"])
	}
}

// 색 컬럼이 없는 옛 zip 이나 routes.txt 가 없는 zip 도 오류 없이 빈 표를 준다(색 없이 진행).
func TestLoadWithoutColors(t *testing.T) {
	path := writeZip(t, "route_id,agency_id,route_short_name,route_long_name,route_type\nM_2,A_SEOULMETRO,2호선,2호선,1\n")
	got, err := Load(path)
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	empty := filepath.Join(t.TempDir(), "empty.zip")
	f, err := os.Create(empty)
	if err != nil {
		t.Fatal(err)
	}
	zip.NewWriter(f).Close()
	f.Close()
	if got, err := Load(empty); err != nil || len(got) != 0 {
		t.Fatalf("routes.txt 없는 zip: got=%v err=%v", got, err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "none.zip")); err == nil {
		t.Error("없는 파일인데 오류가 없다")
	}
}
