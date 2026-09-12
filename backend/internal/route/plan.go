// Package route 는 앱의 경로 요청을 OTP 호출로 바꾸고 결과를 결합·정리한다.
package route

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 상수 출처
//   - 서울 bbox: otp/extract_seoul.py 와 동일(서울 행정경계 + 약 4km).
//   - DefaultWalk 1.2 / DefaultBike 3.5 m/s: otp/router-config.json 과 같은 placeholder. speed_profiles 값이 있으면 덮는다.
//   - BeamWidth 3: 구간 분할 시 구간마다 남기는 후보 수. 호출 수 = 1 + 3×(구간 수−1).
//   - DefaultFirst 10: frequencies 기반 중복(출발시각만 다른 같은 경로)이 많아 넉넉히 받아 중복 제거한다.
const (
	MinLon, MinLat, MaxLon, MaxLat = 126.70, 37.38, 127.25, 37.75
	DefaultWalk                    = 1.2
	DefaultBike                    = 3.5
	BeamWidth                      = 3
	DefaultFirst                   = 10
)

// SegmentMode 는 구간별 수단 고정. "" 또는 "any" 는 대중교통+따릉이+도보 전부.
type SegmentMode string

const (
	ModeAny     SegmentMode = "any"
	ModeWalk    SegmentMode = "walk"
	ModeBike    SegmentMode = "bike"    // 따릉이(+도보)
	ModeTransit SegmentMode = "transit" // 대중교통(+도보 접근)
)

type Point struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type PlanRequest struct {
	Origin      Point         `json:"origin"`
	Destination Point         `json:"destination"`
	Via         []Point       `json:"via"`
	Modes       []SegmentMode `json:"segment_modes"` // 길이 = via 수 + 1. 비면 전 구간 any
	Depart      *time.Time    `json:"depart"`        // nil = 지금
	WalkSpeed   float64       `json:"-"`
	BikeSpeed   float64       `json:"-"`
}

type Planner struct {
	OTP interface {
		Plan(context.Context, otp.Request) ([]otp.Itinerary, error)
	}
}

var ErrBadRequest = errors.New("bad request")

// Plan 은 요청을 검증하고 세 갈래로 OTP 를 부른다.
//   - 경유지 없음, 전 구간 any: 한 번 호출.
//   - 경유지 있음, 전 구간 any: via 를 넘긴 한 번 호출(대중교통 탐색에서만 via 가 동작)과, 도보·따릉이만으로 경유지를
//     도는 후보를 위한 구간 분할 탐색을 병렬로 돌려 합친다. 둘 다 경로가 없을 때만 ErrNoRoute.
//   - 구간별 수단 고정: 구간 분할 탐색만.
//
// 결과는 총 소요시간순, leg 서명이 같은 중복은 제거한다.
func (p *Planner) Plan(ctx context.Context, req PlanRequest) ([]otp.Itinerary, error) {
	if err := validate(&req); err != nil {
		return nil, err
	}
	if req.WalkSpeed <= 0 {
		req.WalkSpeed = DefaultWalk
	}
	if req.BikeSpeed <= 0 {
		req.BikeSpeed = DefaultBike
	}
	locked := false
	for _, m := range req.Modes {
		if m != ModeAny {
			locked = true
		}
	}
	var its []otp.Itinerary
	var err error
	switch {
	case locked:
		its, err = p.segmented(ctx, req)
	case len(req.Via) == 0:
		its, err = p.single(ctx, req)
	default:
		its, err = p.viaBoth(ctx, req)
	}
	if err != nil {
		return nil, err
	}
	its = dedupe(its)
	sort.SliceStable(its, func(i, j int) bool { return its[i].Duration < its[j].Duration })
	return its, nil
}

// viaBoth 는 single(via) 과 segmented(any) 를 동시에 돌려 합친다. 한쪽의 ErrNoRoute 는 무시하고, 둘 다 없을 때만 올린다.
func (p *Planner) viaBoth(ctx context.Context, req PlanRequest) ([]otp.Itinerary, error) {
	type result struct {
		its []otp.Itinerary
		err error
	}
	ch := make(chan result, 2)
	go func() { its, err := p.single(ctx, req); ch <- result{its, err} }()
	go func() { its, err := p.segmented(ctx, req); ch <- result{its, err} }()
	var merged []otp.Itinerary
	var noRoute error
	for i := 0; i < 2; i++ {
		r := <-ch
		switch {
		case r.err == nil:
			merged = append(merged, r.its...)
		case errors.Is(r.err, otp.ErrNoRoute):
			noRoute = r.err
		default:
			return nil, r.err
		}
	}
	if len(merged) == 0 {
		if noRoute == nil {
			noRoute = otp.ErrNoRoute
		}
		return nil, noRoute
	}
	return merged, nil
}

func validate(r *PlanRequest) error {
	pts := append([]Point{r.Origin, r.Destination}, r.Via...)
	for _, pt := range pts {
		if pt.Lon < MinLon || pt.Lon > MaxLon || pt.Lat < MinLat || pt.Lat > MaxLat {
			return fmt.Errorf("%w: 좌표가 서울 범위 밖 (%.4f, %.4f)", ErrBadRequest, pt.Lat, pt.Lon)
		}
	}
	if len(r.Via) > 5 {
		return fmt.Errorf("%w: 경유지는 5개까지", ErrBadRequest)
	}
	n := len(r.Via) + 1
	if len(r.Modes) == 0 {
		r.Modes = make([]SegmentMode, n)
	}
	if len(r.Modes) != n {
		return fmt.Errorf("%w: segment_modes 길이는 경유지 수+1 (%d)", ErrBadRequest, n)
	}
	for i, m := range r.Modes {
		switch m {
		case "", ModeAny:
			r.Modes[i] = ModeAny
		case ModeWalk, ModeBike, ModeTransit:
		default:
			return fmt.Errorf("%w: segment_modes 값 %q", ErrBadRequest, m)
		}
	}
	return nil
}

// modesFor 는 구간 수단을 OTP modes 로 바꾼다. bike 는 OTP 제약상 WALK 를 함께 넣어야 하므로(단독 지정 BadRequest 실측)
// 도보 전용 결과가 섞여 나올 수 있다 → satisfies 로 결과를 거른다. transit 은 transitOnly 로 direct 후보를 억제한다.
func modesFor(m SegmentMode) otp.Modes {
	switch m {
	case ModeWalk:
		return otp.Modes{Direct: []string{"WALK"}, Only: true}
	case ModeBike:
		return otp.Modes{Direct: []string{"BICYCLE_RENTAL", "WALK"}, Only: true}
	case ModeTransit:
		return otp.Modes{TransitOnly: true, Transit: &otp.Transit{
			Access: []string{"WALK"}, Egress: []string{"WALK"}, Transfer: []string{"WALK"}}}
	default:
		return otp.Modes{Direct: []string{"WALK", "BICYCLE_RENTAL"}, Transit: &otp.Transit{
			Access: []string{"BICYCLE_RENTAL", "WALK"}, Egress: []string{"BICYCLE_RENTAL", "WALK"}, Transfer: []string{"WALK"}}}
	}
}

func coord(p Point) otp.Coord { return otp.Coord{Lat: p.Lat, Lon: p.Lon} }

// satisfies 는 고정한 수단의 leg 가 실제로 들어 있는지 본다(bike: 대여 자전거 leg, transit: 대중교통 leg).
func satisfies(m SegmentMode, it otp.Itinerary) bool {
	switch m {
	case ModeBike:
		for _, l := range it.Legs {
			if l.RentedBike || l.Mode == "BICYCLE" {
				return true
			}
		}
		return false
	case ModeTransit:
		for _, l := range it.Legs {
			if l.TransitLeg {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// lastVehicle / firstVehicle: 구간 경계 환승 판정용. 도보가 아닌 leg 의 수단, 없으면 "WALK".
func lastVehicle(legs []otp.Leg) string {
	for i := len(legs) - 1; i >= 0; i-- {
		if legs[i].Mode != "WALK" {
			return legs[i].Mode
		}
	}
	return "WALK"
}

func firstVehicle(legs []otp.Leg) string {
	for _, l := range legs {
		if l.Mode != "WALK" {
			return l.Mode
		}
	}
	return "WALK"
}

// single: via 를 넘긴 한 번 호출. via 는 대중교통 탐색에서만 동작한다(실측).
func (p *Planner) single(ctx context.Context, req PlanRequest) ([]otp.Itinerary, error) {
	r := otp.Request{Origin: coord(req.Origin), Destination: coord(req.Destination), Modes: modesFor(ModeAny),
		WalkSpeed: req.WalkSpeed, BikeSpeed: req.BikeSpeed, Depart: req.Depart, First: DefaultFirst}
	for _, v := range req.Via {
		r.Via = append(r.Via, coord(v))
	}
	return p.OTP.Plan(ctx, r)
}

// segmented: 구간마다 top-BeamWidth 후보를 받아 앞 구간 도착시각을 다음 구간 출발시각으로 넘기며 잇는다.
// 결과는 "구간 제약을 순차 적용한 경로"이지 전역 최적이 아니다.
func (p *Planner) segmented(ctx context.Context, req PlanRequest) ([]otp.Itinerary, error) {
	pts := append(append([]Point{req.Origin}, req.Via...), req.Destination)
	type partial struct {
		legs      []otp.Leg
		start     string
		end       time.Time
		endStr    string
		transfers int
		walk      float64
	}
	beam := []partial{{}}
	for i := 0; i < len(pts)-1; i++ {
		var next []partial
		for _, b := range beam {
			r := otp.Request{Origin: coord(pts[i]), Destination: coord(pts[i+1]), Modes: modesFor(req.Modes[i]),
				WalkSpeed: req.WalkSpeed, BikeSpeed: req.BikeSpeed, First: BeamWidth}
			if i == 0 {
				r.Depart = req.Depart
			} else {
				t := b.end
				r.Depart = &t
			}
			its, err := p.OTP.Plan(ctx, r)
			if errors.Is(err, otp.ErrNoRoute) {
				continue
			}
			if err != nil {
				return nil, err
			}
			for _, it := range its {
				if !satisfies(req.Modes[i], it) {
					continue
				}
				end, err := time.Parse(time.RFC3339, it.End)
				if err != nil {
					continue
				}
				np := partial{legs: append(append([]otp.Leg(nil), b.legs...), it.Legs...), start: b.start,
					end: end, endStr: it.End, transfers: b.transfers + it.Transfers, walk: b.walk + it.WalkM}
				if np.start == "" {
					np.start = it.Start
				}
				// 구간 경계 환승: 양쪽 다 도보면 0, 한쪽이라도 탈것이면 +1(같은 수단끼리도 내렸다 다시 탄다).
				if len(b.legs) > 0 && (lastVehicle(b.legs) != "WALK" || firstVehicle(it.Legs) != "WALK") {
					np.transfers++
				}
				next = append(next, np)
			}
		}
		if len(next) == 0 {
			return nil, fmt.Errorf("%w: 구간 %d", otp.ErrNoRoute, i+1)
		}
		sort.SliceStable(next, func(a, b int) bool { return next[a].end.Before(next[b].end) })
		if len(next) > BeamWidth {
			next = next[:BeamWidth]
		}
		beam = next
	}
	out := make([]otp.Itinerary, 0, len(beam))
	for _, b := range beam {
		st, err := time.Parse(time.RFC3339, b.start)
		if err != nil {
			continue
		}
		out = append(out, otp.Itinerary{Start: b.start, End: b.endStr, Duration: b.end.Sub(st).Seconds(),
			Transfers: b.transfers, WalkM: b.walk, Legs: b.legs})
	}
	return out, nil
}

// dedupe 는 leg 서명(수단·노선·출발지·도착지 열)이 같은 itinerary 중 가장 이른 출발만 남긴다.
// frequencies 기반 버스는 출발시각만 1분씩 다른 같은 경로를 여러 개 내기 때문이다(실측).
func dedupe(its []otp.Itinerary) []otp.Itinerary {
	seen := map[string]int{}
	var out []otp.Itinerary
	for _, it := range its {
		var sb strings.Builder
		for _, l := range it.Legs {
			sb.WriteString(l.Mode + "|" + l.Route + "|" + l.FromName + "|" + l.ToName + ";")
		}
		sig := sb.String()
		if idx, ok := seen[sig]; ok {
			if it.Start < out[idx].Start {
				out[idx] = it
			}
			continue
		}
		seen[sig] = len(out)
		out = append(out, it)
	}
	return out
}
