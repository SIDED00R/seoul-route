package seoulbus

import (
	"os"
	"path/filepath"
	"testing"
)

// 캐시가 있으면 API 를 부르지 않고 그 본문을 읽는다. 점은 no 순으로 정렬한다.
func TestRoutePathFromCache(t *testing.T) {
	dir := t.TempDir()
	body := `{"msgHeader":{"headerCd":"0"},"msgBody":{"itemList":[` +
		`{"no":"10","gpsX":"127.002","gpsY":"37.502"},{"no":"2","gpsX":"127.001","gpsY":"37.501"},` +
		`{"no":"1","gpsX":"127.000","gpsY":"37.500"}]}}`
	os.WriteFile(filepath.Join(dir, "path_100.json"), []byte(body), 0o644)
	got, err := New("", dir).RoutePath("100")
	if err != nil || len(got) != 3 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if got[0].No != "1" || got[1].No != "2" || got[2].No != "10" || got[2].Lon != "127.002" || got[2].Lat != "37.502" {
		t.Errorf("순서·값: %+v", got)
	}
	empty := `{"msgHeader":{"headerCd":"4"},"msgBody":{"itemList":null}}`
	os.WriteFile(filepath.Join(dir, "path_200.json"), []byte(empty), 0o644)
	if got, err := New("", dir).RoutePath("200"); err != nil || len(got) != 0 {
		t.Errorf("경로 없는 노선: got=%+v err=%v", got, err)
	}
}
