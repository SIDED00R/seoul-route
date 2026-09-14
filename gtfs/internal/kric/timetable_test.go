package kric

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// 행이 열차 순이 아니어도 (노선, 요일, 열차번호)로 묶어 시각순으로 정렬한다. 자정 이후는 24:xx, 중복 정차는 뒤 것을 버린다.
func TestLoadGroupsAndSorts(t *testing.T) {
	dir := t.TempDir()
	st := write(t, dir, "s.csv", "line,opr,ln_cd,ln_name,stin_cd,order,name,lat,lon\n"+
		"KJ,KR,K4,경의중앙,K112,3,서빙고,37.5196,126.9884\n"+
		"KJ,KR,K4,경의중앙,K110,1,용산,37.5299,126.9648\n"+
		"KJ,KR,K4,경의중앙,K111,2,이촌,37.5225,126.9738\n"+
		"WS,UI,UI,우이신설,S110,1,북한산우이,37.663,127.012\n")
	tt := write(t, dir, "t.csv", "line,day,trn_no,stin_cd,arv,dep,org,tmn\n"+
		"KJ,8,K5003,K112,050830,050900,K110,K137\n"+
		"KJ,8,K5003,K110,,050100,K110,K137\n"+
		"KJ,8,K5003,K111,050400,050430,K110,K137\n"+
		"KJ,8,K5003,K111,050400,050430,K110,K137\n"+ // 같은 역 중복
		"KJ,9,K5166,K112,000500,000530,K126,K110\n"+ // 자정 이후
		"KJ,9,K5166,K110,001100,,K126,K110\n"+
		"WS,7,1508,S110,,060000,S110,S122\n"+
		"XX,8,1,K110,,050000,K110,K111\n") // 역 파일에 없는 노선은 무시
	got, err := Load(st, tt)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 2 || got.NRows != 7 || got.NDup != 1 || len(got.Trains) != 3 {
		t.Fatalf("lines=%d rows=%d dup=%d trains=%d", len(got.Lines), got.NRows, got.NDup, len(got.Trains))
	}
	kj := got.Lines["KJ"]
	if kj.Opr != "KR" || kj.Name != "경의중앙" || kj.HasSat || len(kj.Stations) != 3 || kj.Stations[0].Code != "K110" ||
		kj.Stations[2].Name != "서빙고" || kj.Stations[1].Lat != 37.5225 {
		t.Fatalf("line=%+v", kj)
	}
	if !got.Lines["WS"].HasSat {
		t.Fatal("우이신설은 토요일 시각표가 있다")
	}
	a := got.Trains[0]
	if a.Line != "KJ" || a.Day != "8" || a.No != "K5003" || a.Org != "K110" || a.Tmn != "K137" || len(a.Stops) != 3 ||
		a.Stops[0].Code != "K110" || a.Stops[0].Arr != "" || a.Stops[0].Dep != "05:01:00" ||
		a.Stops[1].Code != "K111" || a.Stops[2].Arr != "05:08:30" {
		t.Fatalf("train=%+v", a)
	}
	b := got.Trains[1]
	if b.Day != "9" || b.Stops[0].Dep != "24:05:30" || b.Stops[1].Arr != "24:11:00" || b.Stops[1].Dep != "" {
		t.Fatalf("자정 이후 열차: %+v", b)
	}
	if got.Trains[2].Line != "WS" || got.Trains[2].Day != "7" {
		t.Fatalf("정렬: %+v", got.Trains[2])
	}
	if _, err := Load(write(t, dir, "bad.csv", "line,opr\n"), tt); err == nil {
		t.Fatal("필수 열이 없으면 오류")
	}
}
