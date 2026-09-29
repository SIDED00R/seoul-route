package build

import (
	"sort"
	"strconv"
	"strings"
)

// 2호선 순환 열차 이어 타기 규칙.
//   - 서울교통공사 시각표는 2호선 순환 열차를 성수에서 끊는다: 성수 도착으로 끝나는 trip 과 성수 출발로 시작하는 trip.
//     같은 열차가 성수에 섰다가 같은 방향으로 계속 가므로 두 trip 을 같은 block_id 로 묶는다. OTP 는 같은 block 의
//     이어지는 trip 을 내리지 않고 계속 타는 것(interline, stay-seated)으로 본다.
//   - 짝: 같은 service 에서 본선으로 성수에 도착한(직전 정차가 뚝섬·건대입구) trip 과, 그 도착 뒤 loopBlockMaxGapSec 안에
//     성수를 출발해 같은 방향 본선으로 가는(뚝섬에서 왔으면 건대입구, 건대입구에서 왔으면 뚝섬) trip. 도착이 이른 trip
//     부터 짝 없는 출발 중 가장 이른 것을 고른다. 짝이 없는 도착(차량기지 입고)은 묶지 않는다.
//   - block_id 는 이어진 trip 묶음의 첫 trip_id 에 "B_" 를 붙인 것.

// loopBlockMaxGapSec: 성수 도착과 이어지는 출발 사이 최대 간격. 출처: 생성 GTFS 2호선 전 요일(2026-09-29) — 본선 성수 도착
// 1,257건의 같은 방향 다음 출발까지 1분 이하 1,089·1~2분 102·2~3분 5·3~5분 37·5분 넘음 11·없음 13(docs/gtfs-generator.md).
// 시각표를 새로 받으면 다시 잰다.
const loopBlockMaxGapSec = 180

// loopTurnNext 는 성수로 들어온 직전 역 → 같은 방향으로 이어지는 다음 역.
var loopTurnNext = map[string]string{"뚝섬": "건대입구", "건대입구": "뚝섬"}

type loopStats struct {
	Links    int // 이어 붙인 trip 쌍
	Blocks   int // block_id 수
	Unpaired int // 본선으로 성수에 도착했지만 이어지는 출발이 없는 trip
}

type loopEnd struct {
	trip, service, via string // via: 도착은 직전 역, 출발은 다음 역
	sec                int
}

// line2LoopBlocks 는 2호선 trip(route M_2) 중 성수에서 이어지는 것에 block_id 를 붙인다. trips 는
// [route_id, service_id, trip_id, …], stopTimes 는 [trip_id, arrival, departure, stop_id, …] 행(trip 안에서 정차 순),
// stopName 은 stop_id → 역 이름("성수(2호선)"). 반환은 trip_id → block_id.
func line2LoopBlocks(trips, stopTimes [][]string, stopName map[string]string) (map[string]string, loopStats) {
	service := map[string]string{}
	for _, t := range trips {
		if t[0] == "M_2" {
			service[t[2]] = t[1]
		}
	}
	rows := map[string][][]string{}
	for _, r := range stopTimes {
		if _, ok := service[r[0]]; ok {
			rows[r[0]] = append(rows[r[0]], r)
		}
	}
	base := func(stopID string) string {
		n := stopName[stopID]
		if i := strings.Index(n, "("); i >= 0 {
			n = n[:i]
		}
		return strings.TrimSpace(n)
	}
	var arrivals, departures []loopEnd
	for trip, rs := range rows {
		if len(rs) < 2 {
			continue
		}
		last, prev := rs[len(rs)-1], rs[len(rs)-2]
		if base(last[3]) == "성수" && loopTurnNext[base(prev[3])] != "" {
			if s, ok := gtfsSec(last[1]); ok {
				arrivals = append(arrivals, loopEnd{trip, service[trip], base(prev[3]), s})
			}
		}
		first, next := rs[0], rs[1]
		if base(first[3]) == "성수" && loopTurnNext[base(next[3])] != "" {
			if s, ok := gtfsSec(first[2]); ok {
				departures = append(departures, loopEnd{trip, service[trip], base(next[3]), s})
			}
		}
	}
	byTime := func(es []loopEnd) {
		sort.Slice(es, func(i, j int) bool {
			if es[i].sec != es[j].sec {
				return es[i].sec < es[j].sec
			}
			return es[i].trip < es[j].trip
		})
	}
	byTime(arrivals)
	byTime(departures)
	var st loopStats
	next := map[string]string{} // 도착 trip → 이어지는 출발 trip
	used := map[string]bool{}
	for _, a := range arrivals {
		want := loopTurnNext[a.via]
		found := false
		for _, d := range departures {
			if d.sec < a.sec {
				continue
			}
			if d.sec-a.sec > loopBlockMaxGapSec {
				break
			}
			if !used[d.trip] && d.service == a.service && d.via == want {
				next[a.trip], used[d.trip], found = d.trip, true, true
				st.Links++
				break
			}
		}
		if !found {
			st.Unpaired++
		}
	}
	blocks := map[string]string{}
	var heads []string
	for a := range next {
		if !used[a] { // 다른 trip 에서 이어지지 않은 trip 이 묶음의 시작
			heads = append(heads, a)
		}
	}
	sort.Strings(heads)
	for _, h := range heads {
		id := "B_" + h
		for t := h; t != ""; t = next[t] {
			blocks[t] = id
		}
		st.Blocks++
	}
	return blocks, st
}

// gtfsSec 는 GTFS 시각("25:03:30")을 초로 읽는다.
func gtfsSec(s string) (int, bool) {
	p := strings.Split(strings.TrimSpace(s), ":")
	if len(p) != 3 {
		return 0, false
	}
	h, e1 := strconv.Atoi(p[0])
	m, e2 := strconv.Atoi(p[1])
	x, e3 := strconv.Atoi(p[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return 0, false
	}
	return h*3600 + m*60 + x, true
}
