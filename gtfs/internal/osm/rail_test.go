package osm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRailWays(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.csv")
	os.WriteFile(p, []byte("ref,way_id,seq,node_id,lat,lon\n"+
		"2,10,0,100,37.500000,127.000000\n2,10,1,101,37.501000,127.000000\n"+
		"2,11,0,101,37.501000,127.000000\n2,11,1,102,37.502000,127.000000\n"+
		"경의·중앙,10,0,100,37.500000,127.000000\n경의·중앙,10,1,101,37.501000,127.000000\n"), 0o644)
	got, err := LoadRailWays(p)
	if err != nil || len(got) != 3 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if got[0].Ref != "2" || got[0].ID != 10 || len(got[0].Nodes) != 2 || got[0].Nodes[1].ID != 101 ||
		got[0].Nodes[1].Lat != 37.501 {
		t.Errorf("첫 선로=%+v", got[0])
	}
	if got[1].ID != 11 || got[2].Ref != "경의·중앙" || got[2].ID != 10 { // 같은 선로가 노선마다 따로 온다
		t.Errorf("선로 구분: %+v", got)
	}
	if _, err := LoadRailWays(filepath.Join(t.TempDir(), "none.csv")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("없는 파일은 ErrNotExist: %v", err)
	}
	os.WriteFile(p, []byte("ref,way_id,lat,lon\n2,10,1,2\n"), 0o644)
	if _, err := LoadRailWays(p); err == nil {
		t.Fatal("node_id 열이 없으면 오류")
	}
	os.WriteFile(p, []byte("ref,way_id,seq,node_id,lat,lon\n2,x,0,100,37.5,127.0\n"), 0o644)
	if _, err := LoadRailWays(p); err == nil {
		t.Fatal("숫자가 아니면 오류")
	}
}
