package route

import (
	"context"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 신호 횡단보도 대기(crossing.Index)를 후보에 반영하는 규칙.
//   - 도보·자전거 leg 마다 폴리라인이 지나는 신호 횡단보도 수 × 기대 대기(crossing.ExpectedWaitSec)를 그 leg 와 여정의
//     Duration 에 더한다. leg 의 Start/End 문자열은 OTP 시간표 값 그대로 둔다. 여정 End 는 마지막 대중교통 leg
//     뒤(대중교통이 없으면 전 구간)의 도보·자전거 대기만큼 늦춘다.
//   - 탑승 직전까지 이어진 도보·자전거 leg 들(따릉이 접근은 도보+자전거+도보)의 대기 합이 여유(탑승 출발 − 마지막 leg 끝)를
//     넘기면 그 탑승을 놓친다. 그 지점에서 늦어진 시각으로 한 번 다시 탐색해 뒤 구간을 갈아 끼운다(Replanned).
//     탑승을 놓치는 후보 중 점수순 상위 ReplanMax 개에만 하고, 재탐색이 실패하면 후보를 그대로 둔다.
//   - 경유지(Via) 요청은 재탐색하지 않는다. 대기 가산은 그대로 한다.
//   - 갈아 끼운 뒤 구간의 도보 leg 에도 대기를 더하되 재탐색은 다시 하지 않는다(1회).
//   - 재탐색은 놓친 탑승 정류장(역 ID)에서 시작한다.
//   - 도착지가 역 ID 로 앵커링됐고 재탐색 결과가 승강장에서 내리는 지하철·철도로 끝나면 이탈 시간(StationExitSec)을
//     다시 붙인다 — tail 의 End 가 applyStationSlack 이 붙인 값을 덮어쓴다.
//   - 재탐색 결과의 대중교통 열(노선·탑승 정류장)이 탑승을 놓치지 않는 다른 후보와 같으면 버린다(열등 복제).
//   - 서로 다른 원래 후보가 재탐색 뒤 같은 대중교통 열·같은 다음 정차역이 되면 점수(score)가 가장 좋은 하나만 남긴다.
//     같은 역·같은 노선이라도 다음 정차역이 다르면(순환선 내선·외선) 둘 다 남긴다.
//   - 역 안 환승 통로 도보(Leg.InStation: 양끝이 같은 부모역의 정류장이고 역 출입구를 지나지 않는다)는 횡단보도를 세지
//     않는다.
const ReplanMax = 3

// Crossings 는 폴리라인이 지나는 신호 횡단보도 수를 센다(crossing.Index).
type Crossings interface {
	Count(polyline string) int
}

func walksOrBikes(l otp.Leg) bool { return l.Mode == "WALK" || l.Mode == "BICYCLE" }

// addCrossingWaits 는 여정의 도보·자전거 leg 에 대기를 더하고, 재탐색이 필요한 leg 인덱스(탑승 직전 leg, 없으면 −1)와
// 그 탑승 앞에 연속된 도보·자전거 leg 들의 대기 합을 돌려준다.
func addCrossingWaits(it *otp.Itinerary, cx Crossings, waitPer float64) (int, float64) {
	needReplan, needWait, acc := -1, 0.0, 0.0
	lastTransit := -1
	for i, l := range it.Legs {
		if l.TransitLeg {
			lastTransit = i
		}
	}
	for i := range it.Legs {
		l := &it.Legs[i]
		if !walksOrBikes(*l) {
			acc = 0
			continue
		}
		if l.Crossings == 0 && !l.InStation {
			if n := cx.Count(l.Polyline); n > 0 {
				wait := float64(n) * waitPer
				l.Crossings, l.CrossingWait = n, wait
				l.Duration += wait
				it.Duration += wait
				it.CrossingWait += wait
				if i > lastTransit {
					if end, err := time.Parse(time.RFC3339, it.End); err == nil {
						it.End = end.Add(time.Duration(wait * float64(time.Second))).Format(time.RFC3339)
					}
				}
			}
		}
		acc += l.CrossingWait
		if i == len(it.Legs)-1 || needReplan >= 0 || acc == 0 || !it.Legs[i+1].TransitLeg {
			continue
		}
		if slack, ok := slackSec(l.End, it.Legs[i+1].Start); ok && acc > slack {
			needReplan, needWait = i, acc
		}
	}
	return needReplan, needWait
}

func slackSec(walkEnd, boardStart string) (float64, bool) {
	e, err1 := time.Parse(time.RFC3339, walkEnd)
	s, err2 := time.Parse(time.RFC3339, boardStart)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return s.Sub(e).Seconds(), true
}

// applyCrossings 는 전 후보에 대기를 더하고, 탑승을 놓치는 후보 중 점수순 상위 ReplanMax 개를 다시 탐색한다.
// 재탐색 결과가 다른 후보의 열등 복제면 뺀 목록을 돌려준다. destAnchored 는 Plan 이 applyStationSlack 에 준 값과 같다.
func (p *Planner) applyCrossings(ctx context.Context, its []otp.Itinerary, req PlanRequest, waitPer float64,
	destAnchored bool) []otp.Itinerary {
	replanAt := make([]int, len(its))
	replanWait := make([]float64, len(its))
	end0 := make([]string, len(its))
	for i := range its {
		end0[i] = its[i].End
		replanAt[i], replanWait[i] = addCrossingWaits(&its[i], p.Crossings, waitPer)
	}
	if req.Arrive != nil { // 재탐색 대신 일찍 출발한다(arrive_by.go)
		out := its[:0]
		for i := range its {
			if arriveCrossings(&its[i], replanAt[i], replanWait[i], end0[i]) {
				out = append(out, its[i])
			}
		}
		return out
	}
	if len(req.Via) > 0 {
		return its
	}
	// 재탐색 대상은 탑승을 놓치는 후보 중 점수순 상위 ReplanMax. 정렬은 하지 않고(호출자가 나중에 rank) 상위 인덱스만 고른다.
	order := make([]int, len(its))
	for i := range order {
		order[i] = i
	}
	sortIdxByScore(its, order)
	done := 0
	for _, i := range order {
		if done >= ReplanMax {
			break
		}
		if replanAt[i] < 0 {
			continue
		}
		done++
		if tail, ok := p.replanFrom(ctx, its[i], replanAt[i], replanWait[i], req, waitPer, destAnchored); ok {
			its[i] = tail
		}
	}
	kept := map[string]bool{}
	bestReplan := map[string]int{} // 대중교통 열+다음 정차역 → 그렇게 타는 재탐색 후보 중 점수가 가장 좋은 인덱스
	for i, it := range its {
		sig := transitSig(it.Legs)
		if !it.Replanned {
			if replanAt[i] < 0 {
				kept[sig] = true
			}
			continue
		}
		dir := sig + nextStops(it.Legs)
		if j, ok := bestReplan[dir]; !ok || score(it) < score(its[j]) {
			bestReplan[dir] = i
		}
	}
	out := its[:0]
	for i, it := range its {
		sig := transitSig(it.Legs)
		if it.Replanned && (kept[sig] || bestReplan[sig+nextStops(it.Legs)] != i) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// transitSig 는 대중교통 leg 만의 서명(노선·탑승 정류장). 도보를 더 돌더라도 같은 차를 타면 같다.
func transitSig(legs []otp.Leg) string {
	s := ""
	for _, l := range legs {
		if l.TransitLeg {
			s += l.Route + "@" + l.FromStopID + ";"
		}
	}
	return s
}

// nextStops 는 대중교통 leg 마다 탑승 뒤 첫 정차역(NextStop)을 이은 것. 같은 역·같은 노선이라도 방향이 다르면 다르다.
func nextStops(legs []otp.Leg) string {
	s := ""
	for _, l := range legs {
		if l.TransitLeg {
			s += l.NextStop + ";"
		}
	}
	return s
}

// replanFrom 은 leg k(탑승 직전 도보) 까지를 살리고, 그 도착 지점에서 누적 대기(wait)만큼 늦은 시각에 목적지까지
// 다시 탐색해 이어 붙인다.
func (p *Planner) replanFrom(ctx context.Context, it otp.Itinerary, k int, wait float64, req PlanRequest,
	waitPer float64, destAnchored bool) (otp.Itinerary, bool) {
	walk := it.Legs[k]
	end, err := time.Parse(time.RFC3339, walk.End)
	if err != nil {
		return it, false
	}
	depart := end.Add(time.Duration(wait * float64(time.Second)))
	// 재탐색 구간은 도보 접근 + 대중교통만.
	// 출발은 놓친 탑승 정류장 ID(있으면)로.
	r := otp.Request{Origin: otp.Coord{Lat: walk.ToLat, Lon: walk.ToLon}, OriginStop: it.Legs[k+1].FromStopID,
		Destination: coord(req.Destination), DestStop: p.anchor(req.Destination), Modes: modesFor(ModeTransit),
		WalkSpeed: req.WalkSpeed, BikeSpeed: req.BikeSpeed, Depart: &depart, First: DefaultFirst}
	cands, err := p.OTP.Plan(ctx, r)
	if err != nil || len(cands) == 0 {
		return it, false
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c.End < best.End {
			best = c
		}
	}
	// 뒤 구간 도보 leg 의 대기도 더한다(재탐색은 다시 하지 않는다).
	addCrossingWaits(&best, p.Crossings, waitPer)
	out := it
	out.Legs = append(append([]otp.Leg(nil), it.Legs[:k+1]...), best.Legs...)
	out.End = best.End
	if destAnchored && alightsStationLast(best) {
		if e, err := time.Parse(time.RFC3339, best.End); err == nil {
			out.End = e.Add(time.Duration(StationExitSec) * time.Second).Format(time.RFC3339)
		}
	}
	if start, err := time.Parse(time.RFC3339, it.Start); err == nil {
		if e, err := time.Parse(time.RFC3339, out.End); err == nil {
			out.Duration = e.Sub(start).Seconds()
		}
	}
	out.CrossingWait, out.WalkM, out.Transfers = 0, 0, -1
	for _, l := range out.Legs {
		out.CrossingWait += l.CrossingWait
		if l.Mode == "WALK" {
			out.WalkM += l.Distance
		}
		if l.TransitLeg {
			out.Transfers++
		}
	}
	if out.Transfers < 0 {
		out.Transfers = 0
	}
	out.Replanned = true
	return out, true
}

func sortIdxByScore(its []otp.Itinerary, idx []int) {
	for i := 1; i < len(idx); i++ {
		for j := i; j > 0 && score(its[idx[j]]) < score(its[idx[j-1]]); j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}
}
