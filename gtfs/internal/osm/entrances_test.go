package osm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEntrances(t *testing.T) {
	p := filepath.Join(t.TempDir(), "e.csv")
	os.WriteFile(p, []byte("lat,lon,ref,name\n37.497900,127.027600,3,강남역 3번출구\n37.500600,127.036400,,\n"), 0o644)
	got, err := LoadEntrances(p)
	if err != nil || len(got) != 2 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if got[0].Ref != "3" || got[0].Name != "강남역 3번출구" || got[0].Lat != 37.4979 || got[1].Ref != "" {
		t.Fatalf("rows=%+v", got)
	}
	if _, err := LoadEntrances(filepath.Join(t.TempDir(), "none.csv")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("없는 파일은 ErrNotExist: %v", err)
	}
	os.WriteFile(p, []byte("lat,lon\n1,2\n"), 0o644)
	if _, err := LoadEntrances(p); err == nil {
		t.Fatal("ref·name 열이 없으면 오류")
	}
}
