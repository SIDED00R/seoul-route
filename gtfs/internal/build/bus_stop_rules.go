package build

import (
	"sort"
	"strconv"
	"strings"
)

// busClockTolSec: 정류장별 첫차·막차 통과시각(API)을 믿는 범위. 모델 시각(노선 첫차·막차 + 누적 소요)과 이보다 더 다르면
// 자료 오류로 보고 쓰지 않는다. 2026-09-29 버스 2,040 방향 대조에서 막차 값 60,070개 중 3.8%, 첫차 값 60,097개 중
// 3.6% 가 이 범위 밖이다.
const busClockTolSec = 3 * 3600

// busNoBoarding 은 노선 정류장 목록의 가상·미정차 지점이다. 차가 지나기만 하는 곳이라 타고 내릴 수 없다
// ("숭례문(가상)", "부천IC출입(미정차)").
func busNoBoarding(name string) bool {
	return strings.Contains(name, "(가상)") || strings.Contains(name, "(미정차)")
}

// busStopClock 은 정류장별 통과시각 "HH:MM" 을 초로 읽는다. 값이 없으면(":"·빈 값) false. dayStart(노선 첫차, 초)보다
// 이르면 자정을 넘긴 시각으로 보고 24시간을 더한다.
func busStopClock(s string, dayStart int) (int, bool) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return 0, false
	}
	hh, e1 := strconv.Atoi(h)
	mm, e2 := strconv.Atoi(m)
	if e1 != nil || e2 != nil || hh < 0 || hh > 29 || mm < 0 || mm > 59 {
		return 0, false
	}
	v := hh*3600 + mm*60
	if v < dayStart {
		v += 24 * 3600
	}
	return v, true
}

// busWindow 는 배차 trip 의 운행 시간대(첫 기록 정류장 출발 기준, [start, end))와 막차 trip 의 정류장별 시각을 정류장별
// 실제 첫차·막차에 맞춘다. rel[n] 은 첫 기록 정류장부터 n 번째 기록 정류장까지 모델 소요, 모델 첫차·막차는
// first+off+rel[n]·last+off+rel[n] 이다. 실제 − 모델 차이의 방향 안 중앙값만큼 옮긴다: 막차가 이르면 end 를 당기고
// 첫차가 늦으면 start 를 늦춘다(시간대를 넓히지는 않는다). 막차 trip 은 실제 막차를 쓰고, 실제 값이 없는 정류장은 모델
// 시각에 막차 이동량을 더하되 뒤 정류장의 실제 막차보다 늦지 않게 당긴다. 그래도 남는 역행(실제 값끼리 어긋남)은 앞
// 정류장 시각으로 맞춘다. begin·last 는 정류장별 API 값(ok 가 false 면 없음).
func busWindow(first, last, off int, rel []int, begin, lastTm []int,
	beginOK, lastOK []bool) (start, end int, lastAbs []int) {
	lastAbs = make([]int, len(rel))
	real := make([]bool, len(rel))
	var dLast, dBegin []int
	for n := range rel {
		if d := lastTm[n] - (last + off + rel[n]); lastOK[n] && abs(d) <= busClockTolSec {
			lastAbs[n], real[n] = lastTm[n], true
			dLast = append(dLast, d)
		}
		if d := begin[n] - (first + off + rel[n]); beginOK[n] && abs(d) <= busClockTolSec {
			dBegin = append(dBegin, d)
		}
	}
	shift := min(median(dLast), 0)
	start, end = first+off+max(median(dBegin), 0), last+off+shift
	for n := range rel {
		if !real[n] {
			lastAbs[n] = last + off + rel[n] + shift
		}
	}
	for n := len(rel) - 2; n >= 0; n-- {
		if !real[n] && lastAbs[n] > lastAbs[n+1] {
			lastAbs[n] = lastAbs[n+1]
		}
	}
	for n := 1; n < len(rel); n++ {
		lastAbs[n] = max(lastAbs[n], lastAbs[n-1])
	}
	return start, end, lastAbs
}

// median 은 xs 를 정렬해 가운데 값(개수가 짝수면 위쪽)을 돌려준다. 비어 있으면 0.
func median(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	sort.Ints(xs)
	return xs[len(xs)/2]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
