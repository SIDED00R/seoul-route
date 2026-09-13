package seoulmetro

import (
	"os"
	"path/filepath"
	"testing"
)

// 행 순서가 뒤섞여 있어도 (호선, 요일, 열차코드) 로 묶고 시각순으로 정렬한다. 기점은 도착 없음, 종점은 출발 없음. 24:xx 는 23:xx 뒤.
func TestLoadGroupsAndSorts(t *testing.T) {
	csv := "\xef\xbb\xbf\"고유번호\",\"호선\",\"역사코드\",\"역사명\",\"주중주말\",\"방향\",\"급행여부\",\"열차코드\"," +
		"\"열차도착시간\",\"열차출발시간\",\"출발역\",\"도착역\"\n" +
		"1,\"7\",\"2732\",강남구청,DAY,DOWN,\"0\",7006,\"05:41:30\",\"05:41:50\",청담,온수\n" +
		"2,\"7\",\"2731\",청담,DAY,DOWN,\"0\",7006,,\"05:39:00\",청담,온수\n" +
		"3,\"7\",\"2733\",학동,DAY,DOWN,\"0\",7006,\"05:43:20\",,청담,온수\n" +
		"4,\"9\",\"4101\",개화,SAT,UP,\"1\",9502,,\"23:58:00\",개화,중앙보훈병원\n" +
		"5,\"9\",\"4103\",김포공항,SAT,UP,\"1\",9502,\"24:03:00\",,개화,중앙보훈병원\n" +
		"6,\"1\",\"1001\",서울역,DAY,DOWN,\"0\",K1001,\"00:00:00\",\"05:31:30\",청량리,인천\n" + // 일반: 도착 결측
		"7,\"1\",\"1002\",남영,DAY,DOWN,\"0\",K1001,\"00:00:00\",\"00:00:00\",청량리,인천\n" + // 둘 다 결측 → 정차 제외
		"8,\"1\",\"1003\",용산,DAY,DOWN,\"0\",K1001,\"05:34:00\",\"00:00:00\",청량리,인천\n" + // 일반: 출발 결측
		"9,\"1\",\"1001\",서울역,DAY,DOWN,\"1\",K1902,,\"06:00:00\",서울역,천안\n" + // 급행 기점
		"10,\"1\",\"1005\",영등포,DAY,DOWN,\"1\",K1902,\"00:00:00\",\"06:10:00\",서울역,천안\n" + // 급행 통과역 → 제외
		"11,\"1\",\"1007\",안양,DAY,DOWN,\"1\",K1902,\"06:20:00\",\"06:20:30\",서울역,천안\n"
	p := filepath.Join(t.TempDir(), "t.csv")
	os.WriteFile(p, []byte(csv), 0o644)
	tt, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if tt.NRows != 11 || len(tt.Trains) != 4 || tt.NNoTime != 1 || tt.NPassing != 1 {
		t.Fatalf("rows=%d trains=%d notime=%d passing=%d", tt.NRows, len(tt.Trains), tt.NNoTime, tt.NPassing)
	}
	k := tt.Trains[0] // 1호선 일반 K1001: 남영(둘 다 결측)은 빠지고 서울역 도착·용산 출발은 빈 문자열(결측)
	if k.Line != "1" || k.Code != "K1001" || len(k.Stops) != 2 || k.Stops[0].Arr != "" || k.Stops[0].Dep != "05:31:30" ||
		k.Stops[1].Arr != "05:34:00" || k.Stops[1].Dep != "" {
		t.Fatalf("일반 열차의 00:00:00 은 결측: %+v", k.Stops)
	}
	x := tt.Trains[1] // 1호선 급행 K1902: 영등포(도착 00:00:00)는 통과역이라 빠진다
	if !x.Express || len(x.Stops) != 2 || x.Stops[0].Code != "1001" || x.Stops[1].Code != "1007" {
		t.Fatalf("급행의 00:00:00 은 통과역: %+v", x.Stops)
	}
	a := tt.Trains[2]
	if a.Line != "7" || a.Day != "DAY" || a.Code != "7006" || a.Dir != "DOWN" || a.Express ||
		a.Origin != "청담" || a.Dest != "온수" {
		t.Fatalf("train=%+v", a)
	}
	if len(a.Stops) != 3 || a.Stops[0].Code != "2731" || a.Stops[1].Code != "2732" || a.Stops[2].Code != "2733" {
		t.Fatalf("정차는 시각순(청담 05:39 → 강남구청 → 학동): %+v", a.Stops)
	}
	if a.Stops[0].Arr != "" || a.Stops[0].Dep != "05:39:00" || a.Stops[2].Dep != "" || a.Stops[2].Arr != "05:43:20" {
		t.Fatalf("기점 도착·종점 출발은 비어야: %+v", a.Stops)
	}
	b := tt.Trains[3]
	if !b.Express || b.Stops[0].Code != "4101" || b.Stops[1].Arr != "24:03:00" {
		t.Fatalf("급행·자정 넘김: %+v", b)
	}
	os.WriteFile(p, []byte("호선,역사코드\n1,2\n"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("필수 열이 없으면 오류")
	}
}
