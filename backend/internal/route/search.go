package route

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// viaBoth 는 OTP의 via 탐색과 구간별 탐색을 함께 실행한다.
func (p *Planner) viaBoth(ctx context.Context, req PlanRequest) ([]otp.Itinerary, error) {
	type result struct {
		itineraries []otp.Itinerary
		err         error
	}
	results := make(chan result, 2)
	go func() {
		its, err := p.single(ctx, req)
		results <- result{its, err}
	}()
	go func() {
		its, err := p.segmented(ctx, req)
		results <- result{its, err}
	}()

	var (
		merged  []otp.Itinerary
		noRoute error
	)
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			merged = append(merged, result.itineraries...)
		case errors.Is(result.err, otp.ErrNoRoute):
			noRoute = result.err
		default:
			return nil, result.err
		}
	}
	if len(merged) > 0 {
		return merged, nil
	}
	if noRoute == nil {
		noRoute = otp.ErrNoRoute
	}
	return nil, noRoute
}

func lastVehicle(legs []otp.Leg) string {
	for i := len(legs) - 1; i >= 0; i-- {
		if legs[i].Mode != "WALK" {
			return legs[i].Mode
		}
	}
	return "WALK"
}

func firstVehicle(legs []otp.Leg) string {
	for _, leg := range legs {
		if leg.Mode != "WALK" {
			return leg.Mode
		}
	}
	return "WALK"
}

// single 은 전체 수단, 지하철 전용, 버스 전용 탐색 결과를 합친다.
func (p *Planner) single(ctx context.Context, req PlanRequest) ([]otp.Itinerary, error) {
	base := otp.Request{
		Origin:      coord(req.Origin),
		Destination: coord(req.Destination),
		WalkSpeed:   req.WalkSpeed,
		BikeSpeed:   req.BikeSpeed,
		Depart:      req.Depart,
		First:       DefaultFirst,
	}
	base.OriginStop, base.DestStop = p.anchor(req.Origin), p.anchor(req.Destination)
	if base.OriginStop != "" {
		base.Depart = entryDepart(req.Depart, p.now())
	}
	for _, via := range req.Via {
		base.Via = append(base.Via, coord(via))
		base.ViaStops = append(base.ViaStops, p.anchor(via))
	}

	variants := []otp.Modes{modesFor(ModeAny), transitOnlyModes("SUBWAY"), transitOnlyModes("BUS")}
	results := make([][]otp.Itinerary, len(variants))
	errs := make([]error, len(variants))
	var group sync.WaitGroup
	for i, modes := range variants {
		group.Add(1)
		go func() {
			defer group.Done()
			request := base
			request.Modes = modes
			results[i], errs[i] = p.OTP.Plan(ctx, request)
		}()
	}
	group.Wait()

	var merged []otp.Itinerary
	for i := range variants {
		if errs[i] != nil && !errors.Is(errs[i], otp.ErrNoRoute) {
			return nil, errs[i]
		}
		merged = append(merged, results[i]...)
	}
	if len(merged) > 0 {
		return merged, nil
	}
	if errs[0] != nil {
		return nil, errs[0]
	}
	return nil, otp.ErrNoRoute
}

func transitOnlyModes(mode string) otp.Modes {
	return otp.Modes{
		Transit: &otp.Transit{
			Access:   []string{"WALK"},
			Egress:   []string{"WALK"},
			Transfer: []string{"WALK"},
			Modes:    []otp.TransitMode{{Mode: mode}},
		},
		TransitOnly: true,
	}
}

// segmented 는 각 구간의 후보를 앞 구간 도착 시각에 이어 붙이는 빔 탐색이다.
func (p *Planner) segmented(ctx context.Context, req PlanRequest) ([]otp.Itinerary, error) {
	points := append(append([]Point{req.Origin}, req.Via...), req.Destination)
	beam := []partial{{}}
	for segment := 0; segment < len(points)-1; segment++ {
		results := make([][]partial, len(beam))
		errs := make([]error, len(beam))
		var group sync.WaitGroup
		for i, candidate := range beam {
			group.Add(1)
			go func() {
				defer group.Done()
				results[i], errs[i] = p.extend(ctx, req, points, segment, candidate)
			}()
		}
		group.Wait()

		var next []partial
		for i := range beam {
			if errs[i] != nil && !errors.Is(errs[i], otp.ErrNoRoute) {
				return nil, errs[i]
			}
			next = append(next, results[i]...)
		}
		if len(next) == 0 {
			return nil, fmt.Errorf("%w: 구간 %d", otp.ErrNoRoute, segment+1)
		}
		beam = prune(next)
	}

	out := make([]otp.Itinerary, 0, len(beam))
	for _, candidate := range beam {
		start, err := time.Parse(time.RFC3339, candidate.start)
		if err != nil {
			continue
		}
		out = append(out, otp.Itinerary{
			Start: candidate.start, End: candidate.endStr, Duration: candidate.end.Sub(start).Seconds(),
			Transfers: candidate.transfers, WalkM: candidate.walk, Legs: candidate.legs,
		})
	}
	return out, nil
}

func (p *Planner) extend(ctx context.Context, req PlanRequest, points []Point, segment int, candidate partial) ([]partial, error) {
	request := otp.Request{
		Origin:      coord(points[segment]),
		Destination: coord(points[segment+1]),
		Modes:       modesFor(req.Modes[segment]),
		WalkSpeed:   req.WalkSpeed,
		BikeSpeed:   req.BikeSpeed,
		First:       DefaultFirst,
	}
	if anchorsSegment(req.Modes[segment]) {
		request.OriginStop = p.anchor(points[segment])
		request.DestStop = p.anchor(points[segment+1])
	}
	if segment == 0 {
		request.Depart = req.Depart
		if request.OriginStop != "" {
			request.Depart = entryDepart(req.Depart, p.now())
		}
	} else {
		request.Depart = &candidate.end
	}

	itineraries, err := p.OTP.Plan(ctx, request)
	if err != nil {
		return nil, err
	}
	var out []partial
	for _, itinerary := range itineraries {
		if !satisfies(req.Modes[segment], itinerary) {
			continue
		}
		end, err := time.Parse(time.RFC3339, itinerary.End)
		if err != nil {
			continue
		}
		next := partial{
			legs:      append(append([]otp.Leg(nil), candidate.legs...), itinerary.Legs...),
			start:     candidate.start,
			end:       end,
			endStr:    itinerary.End,
			transfers: candidate.transfers + itinerary.Transfers,
			walk:      candidate.walk + itinerary.WalkM,
		}
		if next.start == "" {
			next.start = itinerary.Start
		}
		if len(candidate.legs) > 0 && (lastVehicle(candidate.legs) != "WALK" || firstVehicle(itinerary.Legs) != "WALK") {
			next.transfers++
		}
		out = append(out, next)
	}
	return out, nil
}

type partial struct {
	legs      []otp.Leg
	start     string
	end       time.Time
	endStr    string
	transfers int
	walk      float64
}

// prune 은 중복을 제거하고 도착 시각·환승·도보 거리에서 지배되지 않는 후보를 우선한다.
func prune(candidates []partial) []partial {
	best := map[string]int{}
	var unique []partial
	for _, candidate := range candidates {
		sig := legSig(candidate.legs)
		if index, ok := best[sig]; ok {
			if candidate.end.Before(unique[index].end) {
				unique[index] = candidate
			}
			continue
		}
		best[sig] = len(unique)
		unique = append(unique, candidate)
	}
	sort.SliceStable(unique, func(i, j int) bool { return unique[i].end.Before(unique[j].end) })

	var out []partial
	taken := make([]bool, len(unique))
	for i, candidate := range unique {
		dominated := false
		for _, kept := range out {
			if kept.transfers <= candidate.transfers && kept.walk <= candidate.walk {
				dominated = true
				break
			}
		}
		if !dominated && len(out) < BeamWidth {
			out = append(out, candidate)
			taken[i] = true
		}
	}
	for i, candidate := range unique {
		if len(out) == BeamWidth {
			break
		}
		if !taken[i] {
			out = append(out, candidate)
		}
	}
	return out
}
