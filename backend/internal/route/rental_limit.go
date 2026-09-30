package route

import (
	"context"
	"math"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/crossing"
	"github.com/SIDED00R/seoul-route/backend/internal/geo"
	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 따릉이 대여 한도(PlanRequest.BikeLimitMin) 규칙.
//   - 대여 1회 = 이어진 따릉이 대여 leg 묶음. 그 시간(leg 소요 + 경로선이 지나는 신호 횡단보도 기대 대기)이
//     한도 − RentalMarginSec 을 넘으면 한도 초과다.
//   - 한도를 넘는 후보 중 대중교통이 없고 가장 일찍 도착하는 하나를, 그 대여의 경로선 SplitStationM 안에서 한도보다
//     SplitSlackSec 앞에 닿는 가장 먼 대여소에서 반납하고 다시 빌리는 후보로 바꾼다: 구간 출발 → 대여소, 대여소 → 구간
//     도착을 따릉이 수단으로 다시 탐색해 잇는다. 뒤쪽도 넘으면 MaxRentalSplits 번까지 되풀이한다.
//   - 대중교통이 섞인 후보가 한도를 넘으면 뺀다. 나눌 대여소를 못 찾거나 다시 탐색이 실패해도 뺀다.
//   - 반납 후 재대여는 환승으로 세지 않는다.
//   - OTP via 후보(경유지 있고 체류 없음)는 나누지 않고 한도를 넘으면 빼기만 한다. 구간별 탐색이 구간마다 나눈 후보를 낸다.

const (
	// RentalMarginSec: 한도에서 뺄 여유. 반납·대여 조작과 예상 소요 오차를 흡수하는 초기 제품값(2026-09-28),
	// 실제 대여 기록이 쌓이면 재보정한다.
	RentalMarginSec = 300.0
	// SplitStationM: 반납할 대여소를 찾는 경로선으로부터의 거리.
	SplitStationM = 200.0
	// MaxRentalSplits: 한 구간에서 반납 후 재대여를 넣는 최대 횟수.
	MaxRentalSplits = 3
	// SplitSlackSec·SplitTries: 반납 대여소는 원래 경로에서 한도보다 SplitSlackSec 앞에 닿는 곳에서 고르고, 그 대여소까지
	// 다시 탐색한 대여가 한도를 넘으면 SplitSlackSec 씩 더 앞에서 SplitTries 번까지 다시 고른다.
	// 출처: 2026-09-28 개발 스택 실측(발산→천호 따릉이 고정·1시간권). 여유 0 이면 둘째 반납 대여소까지 다시 탐색한 대여가
	// 3568초로 한도 3300초를 넘어 경로 없음, 여유 300초면 대여 4회(42·53·48·10분). 실제 대여 기록이 쌓이면 재보정한다.
	SplitSlackSec = 300.0
	SplitTries    = 3
)

func rentalLimitSec(req PlanRequest) float64 { return float64(req.BikeLimitMin)*60 - RentalMarginSec }

// rentalRuns 는 이어진 따릉이 대여 leg 묶음마다 [첫 leg, 끝 leg] 인덱스.
func rentalRuns(legs []otp.Leg) [][2]int {
	var runs [][2]int
	for i := 0; i < len(legs); i++ {
		if !legs[i].RentedBike {
			continue
		}
		j := i
		for j+1 < len(legs) && legs[j+1].RentedBike {
			j++
		}
		runs = append(runs, [2]int{i, j})
		i = j
	}
	return runs
}

// legRentalSec 는 대여 leg 하나가 쓰는 대여 시간. 횡단보도 대기가 아직 더해지지 않았으면 기대 대기를 더해 센다.
func (p *Planner) legRentalSec(l otp.Leg) float64 {
	sec := l.Duration
	if p.Crossings != nil && l.CrossingWait == 0 {
		sec += float64(p.Crossings.Count(l.Polyline)) * p.CrossingSec
	}
	return sec
}

// overLimit 은 대여 시간이 limit 초를 넘는 첫 대여 묶음.
func (p *Planner) overLimit(legs []otp.Leg, limit float64) ([2]int, bool) {
	for _, run := range rentalRuns(legs) {
		sec := 0.0
		for k := run[0]; k <= run[1]; k++ {
			sec += p.legRentalSec(legs[k])
		}
		if sec > limit {
			return run, true
		}
	}
	return [2]int{}, false
}

func hasTransit(it otp.Itinerary) bool {
	for _, l := range it.Legs {
		if l.TransitLeg {
			return true
		}
	}
	return false
}

// dropLongRentals 는 한도를 넘는 대여가 있는 후보를 뺀다.
func (p *Planner) dropLongRentals(req PlanRequest, its []otp.Itinerary) []otp.Itinerary {
	if req.BikeLimitMin == 0 {
		return its
	}
	limit := rentalLimitSec(req)
	var kept []otp.Itinerary
	for _, it := range its {
		if _, over := p.overLimit(it.Legs, limit); !over {
			kept = append(kept, it)
		}
	}
	return kept
}

// fitRentals 는 from→to 구간 후보에 대여 한도를 적용한다. depart 는 그 구간 탐색의 출발 시각(nil = 지금).
func (p *Planner) fitRentals(ctx context.Context, req PlanRequest, from, to Point, depart *time.Time,
	its []otp.Itinerary) []otp.Itinerary {
	if req.BikeLimitMin == 0 {
		return its
	}
	limit := rentalLimitSec(req)
	var kept []otp.Itinerary
	split := -1
	for i, it := range its {
		if _, over := p.overLimit(it.Legs, limit); !over {
			kept = append(kept, it)
			continue
		}
		if !hasTransit(it) && (split < 0 || it.End < its[split].End) {
			split = i
		}
	}
	if split >= 0 {
		if it, ok := p.splitRental(ctx, req, from, to, depart, its[split], MaxRentalSplits); ok {
			kept = append(kept, it)
		}
	}
	return kept
}

// splitRental 은 it 의 한도 초과 대여를 한도 안에 닿는 대여소에서 끊고, from→대여소·대여소→to 를 따릉이 수단으로
// 다시 탐색해 잇는다. 뒤쪽이 또 넘으면 splits 번까지 되풀이한다.
func (p *Planner) splitRental(ctx context.Context, req PlanRequest, from, to Point, depart *time.Time,
	it otp.Itinerary, splits int) (otp.Itinerary, bool) {
	limit := rentalLimitSec(req)
	run, over := p.overLimit(it.Legs, limit)
	if !over {
		return it, true
	}
	if splits == 0 {
		return otp.Itinerary{}, false
	}
	var (
		stop Point
		head otp.Itinerary
		ok   bool
	)
	for try := 1; try <= SplitTries && !ok; try++ {
		if stop, ok = p.splitStation(it.Legs, run, limit-float64(try)*SplitSlackSec); !ok {
			return otp.Itinerary{}, false
		}
		head, ok = p.fastestBike(ctx, req, from, stop, depart, limit)
	}
	if !ok {
		return otp.Itinerary{}, false
	}
	headEnd, err := time.Parse(time.RFC3339, head.End)
	if err != nil {
		return otp.Itinerary{}, false
	}
	tail, ok := p.fastestBike(ctx, req, stop, to, &headEnd, math.Inf(1))
	if !ok {
		return otp.Itinerary{}, false
	}
	if tail, ok = p.splitRental(ctx, req, stop, to, &headEnd, tail, splits-1); !ok {
		return otp.Itinerary{}, false
	}
	return joinItineraries(head, tail), true
}

// fastestBike 는 from→to 를 따릉이 수단으로 탐색해 대여가 limit 초 안인 후보 중 가장 일찍 도착하는 것.
func (p *Planner) fastestBike(ctx context.Context, req PlanRequest, from, to Point, depart *time.Time,
	limit float64) (otp.Itinerary, bool) {
	its, err := p.OTP.Plan(ctx, otp.Request{Origin: coord(from), Destination: coord(to), Modes: modesFor(ModeBike),
		WalkSpeed: req.WalkSpeed, BikeSpeed: req.BikeSpeed, Depart: depart, First: DefaultFirst})
	if err != nil {
		return otp.Itinerary{}, false
	}
	best := -1
	for i, it := range its {
		if _, over := p.overLimit(it.Legs, limit); over {
			continue
		}
		if best < 0 || it.End < its[best].End {
			best = i
		}
	}
	if best < 0 {
		return otp.Itinerary{}, false
	}
	return its[best], true
}

// splitStation 은 run 대여 묶음의 경로선에서 대여 시작 뒤 limit 초 안에 닿는 지점의 SplitStationM 안에 있는 대여소 중
// 가장 늦게 닿는 곳. 한도의 절반도 못 가서 닿는 곳뿐이면 없다고 본다.
func (p *Planner) splitStation(legs []otp.Leg, run [2]int, limit float64) (Point, bool) {
	if p.Bikes == nil {
		return Point{}, false
	}
	snap, _ := p.Bikes.Current() // 대여소 위치만 쓰므로 오래된 스냅샷이어도 된다
	if snap == nil {
		return Point{}, false
	}
	type mark struct{ lat, lon, sec float64 }
	var marks []mark // 대여 시작부터 걸린 시간 순
	elapsed := 0.0
	for k := run[0]; k <= run[1]; k++ {
		pts := crossing.DecodePolyline(legs[k].Polyline)
		legSec := p.legRentalSec(legs[k])
		total := 0.0
		for i := 1; i < len(pts); i++ {
			total += geo.DistM(pts[i-1].Lat, pts[i-1].Lon, pts[i].Lat, pts[i].Lon)
		}
		along := 0.0
		for i, pt := range pts {
			if i > 0 {
				along += geo.DistM(pts[i-1].Lat, pts[i-1].Lon, pt.Lat, pt.Lon)
			}
			sec := elapsed
			if total > 0 {
				sec += legSec * along / total
			}
			if sec > limit {
				break
			}
			marks = append(marks, mark{pt.Lat, pt.Lon, sec})
		}
		elapsed += legSec
	}
	// SplitStationM 을 위경도 차로 거칠게 먼저 거른다(서울 위도에서 위도 0.002° ≈ 222m, 경도 0.0025° ≈ 221m).
	const dLat, dLon = 0.002, 0.0025
	best, bestSec := -1, limit/2
	for i, s := range snap.Stations {
		for m := len(marks) - 1; m >= 0 && marks[m].sec > bestSec; m-- {
			if math.Abs(marks[m].lat-s.Lat) > dLat || math.Abs(marks[m].lon-s.Lon) > dLon {
				continue
			}
			if geo.DistM(marks[m].lat, marks[m].lon, s.Lat, s.Lon) <= SplitStationM {
				best, bestSec = i, marks[m].sec
				break
			}
		}
	}
	if best < 0 {
		return Point{}, false
	}
	s := snap.Stations[best]
	return Point{Lat: s.Lat, Lon: s.Lon, Name: s.Name}, true
}

// joinItineraries 는 a 다음에 b 를 이은 여정. 반납 후 재대여는 환승으로 세지 않는다.
func joinItineraries(a, b otp.Itinerary) otp.Itinerary {
	out := otp.Itinerary{
		Start: a.Start, End: b.End, Transfers: a.Transfers + b.Transfers, WalkM: a.WalkM + b.WalkM,
		Legs: append(append([]otp.Leg(nil), a.Legs...), b.Legs...),
	}
	start, err1 := time.Parse(time.RFC3339, a.Start)
	end, err2 := time.Parse(time.RFC3339, b.End)
	if err1 == nil && err2 == nil {
		out.Duration = end.Sub(start).Seconds()
	}
	return out
}
