package build

import (
	"archive/zip"
	"path/filepath"
	"strings"
	"testing"
)

func TestBusStopClock(t *testing.T) {
	const dayStart = 4*3600 + 10*60 // 04:10
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"04:20", 4*3600 + 20*60, true},
		{"23:59", 23*3600 + 59*60, true},
		{"00:31", 24*3600 + 31*60, true}, // 노선 첫차보다 이르면 자정 넘김
		{":", 0, false},
		{"", 0, false},
		{"4:x", 0, false},
		{"30:00", 0, false},
	}
	for _, c := range cases {
		if got, ok := busStopClock(c.in, dayStart); got != c.want || ok != c.ok {
			t.Errorf("%q: got %d,%v want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestBusNoBoarding(t *testing.T) {
	for name, want := range map[string]bool{"숭례문(가상)": true, "부천IC출입(미정차)": true, "서울역버스환승센터": false,
		"가상현실체험관": false} {
		if busNoBoarding(name) != want {
			t.Errorf("%q: want %v", name, want)
		}
	}
}

func TestBusWindow(t *testing.T) {
	const first, last, off = 4*3600 + 10*60, 24*3600 + 30*60, 0 // 04:10 ~ 24:30
	rel := []int{0, 74, 212}
	none := []bool{false, false, false}
	zero := []int{0, 0, 0}

	// 자료가 없으면 그대로(모델)
	s, e, la := busWindow(first, last, off, rel, zero, zero, none, none)
	if s != first || e != last || la[0] != last || la[2] != last+212 {
		t.Fatalf("자료 없음: %d %d %v", s, e, la)
	}
	// C 의 실제 막차가 모델(24:33:32)보다 152초 이른 24:31 → end 를 152초 당긴다. 막차 trip 의 C 는 24:31, 실제 값이 없는
	// A·B 는 모델에서 152초 당긴 24:27:28·24:28:42. C 의 실제 첫차가 모델(04:13:32)보다 388초 늦은 04:20 → start 를 388초 늦춘다.
	lastTm := []int{0, 0, 24*3600 + 31*60}
	begin := []int{0, 0, 4*3600 + 20*60}
	on := []bool{false, false, true}
	s, e, la = busWindow(first, last, off, rel, begin, lastTm, on, on)
	if s != first+388 || e != last-152 || la[2] != 24*3600+31*60 || la[1] != last+74-152 || la[0] != last-152 {
		t.Fatalf("좁히기: start=%s end=%s last=%v", fmtTime(s), fmtTime(e), la)
	}
	// 이동량은 정류장별 차이의 중앙값이다: −600·−120·−60초 → end 를 120초 당긴다(가장 이른 −600초가 아니다).
	all := []bool{true, true, true}
	s, e, la = busWindow(first, last, off, rel, zero, []int{last - 600, last + 74 - 120, last + 212 - 60}, none, all)
	if s != first || e != last-120 || la[0] != last-600 || la[1] != last+74-120 || la[2] != last+212-60 {
		t.Errorf("중앙값: start=%s end=%s last=%v", fmtTime(s), fmtTime(e), la)
	}
	// 실제 막차가 모델보다 늦으면 end 를 늘리지 않는다.
	late := []int{last + 60, last + 74 + 120, last + 212 + 300}
	if _, e, _ = busWindow(first, last, off, rel, zero, late, none, all); e != last {
		t.Errorf("늦은 막차로 끝을 늘림: %s", fmtTime(e))
	}
	// 실제 첫차가 모델보다 이르면 시작을 당기지 않는다(노선 첫차 전 가상 버스를 만들지 않는다).
	s, _, _ = busWindow(first, last, off, rel, []int{0, 0, 4*3600 + 11*60}, zero, on, none)
	if s != first {
		t.Errorf("이른 첫차로 시작을 당김: %s", fmtTime(s))
	}
	// 모델과 3시간 넘게 다른 값은 자료 오류로 보고 쓰지 않는다.
	s, e, la = busWindow(first, last, off, rel, []int{0, 0, 9 * 3600}, []int{0, 0, 19 * 3600}, on, on)
	if s != first || e != last || la[2] != last+212 {
		t.Errorf("이상값: %s %s %v", fmtTime(s), fmtTime(e), la)
	}
	// 실제 값끼리 역행하면(B 24:35, C 24:31) 막차 trip 은 앞 정류장보다 이르게 지나지 않는다. 차이 +226·−152초의
	// 중앙값은 위쪽(+226)이라 이동량 0 → A 는 모델 그대로.
	_, _, la = busWindow(first, last, off, rel, zero, []int{0, 24*3600 + 35*60, 24*3600 + 31*60},
		none, []bool{false, true, true})
	if la[0] != last || la[1] != 24*3600+35*60 || la[2] != 24*3600+35*60 {
		t.Errorf("단조: %v", la)
	}
}

// 배선: 가상·미정차 지점은 승하차 불가, 정류장별 막차로 frequencies 끝과 막차 trip 이 바뀐다. 시간대가 없으면 배차 trip 이 없다.
func TestBuildBusStopRules(t *testing.T) {
	r := sampleRoute() // 04:10 ~ 00:30, 누적 0·74·212초
	r.Stops[1].Name = "B(미정차)"
	r.Stops[2].LastTm = "00:31"
	out := filepath.Join(t.TempDir(), "g.zip")
	rep, err := Build(out, []BusRoute{r}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NBusNoBoardingStops != 1 || rep.NBusNarrowedDirections != 1 {
		t.Fatalf("report=%+v", rep)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	st := readTable(t, zr, "stop_times.txt")
	if strings.Join(st[0][6:], ",") != "pickup_type,drop_off_type" {
		t.Fatalf("header=%v", st[0])
	}
	var got []string
	for _, row := range st[1:] {
		got = append(got, row[0][len(row[0])-2:]+"@"+row[3]+"="+row[1]+"/"+row[6]+row[7])
	}
	// 정류장마다 배차 trip(_T)·막차 trip(ST = _LAST 끝 두 글자) 순. 막차 C 는 실제 24:31, 값이 없는 A·B 는 모델에서
	// C 의 차이(−152초)만큼 당긴다.
	want := "_T@BS_106000101=00:00:00/,ST@BS_106000101=24:27:28/,_T@BS_106000100=00:01:14/11," +
		"ST@BS_106000100=24:28:42/11,_T@BS_106000097=00:03:32/,ST@BS_106000097=24:31:00/"
	if strings.Join(got, ",") != want {
		t.Errorf("stop_times\ngot  %s\nwant %s", strings.Join(got, ","), want)
	}
	fq := readTable(t, zr, "frequencies.txt")
	if len(fq) != 2 || fq[1][1] != "04:10:00" || fq[1][2] != "24:27:28" {
		t.Errorf("frequencies=%v", fq)
	}

	// 실제 막차가 첫차 시간대보다 이르면(시간대 없음) 배차 trip 을 넣지 않는다.
	r2 := sampleRoute()
	r2.Route.FirstBus, r2.Route.LastBus = "20260912220000", "20260912223000"
	r2.Stops[2].LastTm = "22:03"
	out2 := filepath.Join(t.TempDir(), "g2.zip")
	if _, err := Build(out2, []BusRoute{r2}, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	zr2, err := zip.OpenReader(out2)
	if err != nil {
		t.Fatal(err)
	}
	defer zr2.Close()
	for _, row := range readTable(t, zr2, "trips.txt")[1:] {
		if strings.Contains(row[2], "_T") {
			t.Errorf("시간대 없는 배차 trip: %v", row)
		}
	}
	if fq := readTable(t, zr2, "frequencies.txt"); len(fq) != 1 {
		t.Errorf("frequencies=%v", fq)
	}
}
