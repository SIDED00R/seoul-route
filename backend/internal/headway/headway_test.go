package headway

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "g.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	f.Close()
	return p
}

// 노선별 배차: trip → route 매핑, 여러 행이면 최소값, BOM 헤더·잘못된 값은 무시.
func TestLoad(t *testing.T) {
	const utf8BOM = "\xef\xbb\xbf"
	p := writeZip(t, map[string]string{
		"trips.txt": utf8BOM + `route_id,service_id,trip_id
B_1,ALL,B_1_T
B_1,ALL,B_1_LAST
B_2,ALL,B_2_T
RR_9,ALL,RR_9_1
`,
		"frequencies.txt": `trip_id,start_time,end_time,headway_secs,exact_times
B_1_T,04:00:00,09:00:00,540,1
B_1_T,09:00:00,23:00:00,720,1
B_2_T,05:00:00,23:00:00,abc,1
X_T,05:00:00,23:00:00,600,1
`,
	})
	h, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if h["B_1"] != 540 || len(h) != 1 {
		t.Fatalf("B_1 은 540(최소), 잘못된 값·미지 trip 은 제외: %v", h)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "none.zip")); err == nil {
		t.Fatal("없는 파일은 오류")
	}
}
