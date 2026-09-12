// Package route 는 앱의 경로 요청을 OTP 호출로 바꾸고 결과를 결합·정리한다.
package route

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
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
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Name string  `json:"name,omitempty"` // 장소명. "…역" 이면 근처 같은 이름 역으로 앵커링한다(anchor.go)
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
	// Realtime 은 "지금 출발" 후보의 첫 탑승 대기를 실시간 도착으로 바꾼다(realtime.Corrector). nil 이면 시간표 값 그대로.
	Realtime interface {
		Adjust(context.Context, []otp.Itinerary) []otp.Itinerary
	}
	Now      func() time.Time // 테스트용 현재 시각. nil 이면 time.Now
	mu       sync.RWMutex
	stations []otp.Station // 앵커링용 부모역 목록(SetStations). 비면 항상 좌표로 요청한다
}

// SetStations 는 부모역 목록을 바꾼다. OTP 가 늦게 뜨는 경우 기동 후 뒤늦게 채우므로 잠근다.
func (p *Planner) SetStations(s []otp.Station) {
	p.mu.Lock()
	p.stations = s
	p.mu.Unlock()
}

var ErrBadRequest = errors.New("bad request")

// Plan 은 요청을 검증하고 세 갈래로 OTP 를 부른다.
//   - 경유지 없음, 전 구간 any: 한 번 호출.
//   - 경유지 있음, 전 구간 any: via 를 넘긴 한 번 호출(대중교통 탐색에서만 via 가 동작)과, 도보·따릉이만으로 경유지를
//     도는 후보를 위한 구간 분할 탐색을 병렬로 돌려 합친다. 둘 다 경로가 없을 때만 ErrNoRoute.
//   - 구간별 수단 고정: 구간 분할 탐색만.
//
// 결과는 leg 서명이 같은 중복을 제거한 뒤 점수순(소요시간 + 환승·대여 페널티, rank 참조)이다.
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
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	setDepartIn(its, req.Depart, now)
	// 실시간은 점수순 상위 후보의 첫 탑승에만 적용한다. 그 전에는 정렬만 하고 컷은 걸지 않는다 — 최선이 실시간으로
	// 늦어지면 컷 경계 밖에 있던 후보가 경쟁력을 얻는데, 먼저 잘라 버리면 되살릴 수 없다.
	// 미래 출발(Depart 지정)은 실시간과 무관하므로 건너뛴다.
	if p.Realtime != nil && req.Depart == nil {
		sortByScore(its)
		its = p.Realtime.Adjust(ctx, its)
		setDepartIn(its, nil, now)
	}
	return rank(its), nil
}

func sortByScore(its []otp.Itinerary) {
	sort.SliceStable(its, func(i, j int) bool { return score(its[i]) < score(its[j]) })
}

// setDepartIn 은 "지금 출발" 요청에서 요청 시각부터 여정 출발까지의 대기(초)를 채운다. 미래 출발이면 0.
func setDepartIn(its []otp.Itinerary, depart *time.Time, now time.Time) {
	for i := range its {
		its[i].DepartIn = 0
		if depart != nil {
			continue
		}
		if st, err := time.Parse(time.RFC3339, its[i].Start); err == nil && st.After(now) {
			its[i].DepartIn = st.Sub(now).Seconds()
		}
	}
}

// 순위 상수(제품 결정 2026-09-12: 환승·수단 전환이 잦은 후보는 뒤로, 도보·따릉이를 우대하지 않는다).
//   - TransferPenaltySec 240: 환승 1회를 4분 손해로 친다. 카카오·구글이 쓰는 값은 비공개라 초기값이며 실사용 후 조정.
//   - RentalPenaltySec 300: 따릉이 대여·반납 수고를 5분으로 친다(OTP 의 대여·반납 1분씩은 소요시간에 이미 포함).
//   - MaxSlowerSec 1800: 최선보다 30분 넘게 느린 후보는 목록에서 뺀다(홍대입구→잠실 따릉이 122분 vs 지하철 38분).
//     기준은 실시간 보정을 거친 뒤의 최선이다(보정 전에 자르면 최선이 늦어져도 후보를 되살릴 수 없다).
const (
	TransferPenaltySec = 240.0
	RentalPenaltySec   = 300.0
	MaxSlowerSec       = 1800.0
)

// rank 는 총 소요(출발 대기 DepartIn + Duration)에 환승·대여 페널티를 더한 점수순으로 정렬하고,
// 최선보다 MaxSlowerSec 넘게 느린 후보를 뺀다.
func rank(its []otp.Itinerary) []otp.Itinerary {
	if len(its) == 0 {
		return its
	}
	best := total(its[0])
	for _, it := range its[1:] {
		if total(it) < best {
			best = total(it)
		}
	}
	kept := its[:0:0]
	for _, it := range its {
		if total(it) <= best+MaxSlowerSec {
			kept = append(kept, it)
		}
	}
	sortByScore(kept)
	return kept
}

// total 은 요청 시각부터 도착까지(초). 지금 출발이면 출발 대기가 포함된다.
func total(it otp.Itinerary) float64 { return it.DepartIn + it.Duration }

func score(it otp.Itinerary) float64 {
	s := total(it) + TransferPenaltySec*float64(it.Transfers)
	for _, l := range it.Legs {
		if l.RentedBike {
			s += RentalPenaltySec
			break
		}
	}
	return s
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

// single: via 를 넘긴 호출. via 는 대중교통 탐색에서만 동작한다(실측).
// 전체 수단 1회 + 지하철만·버스만 1회씩을 병렬로 돌려 합친다. OTP 는 지배되지 않는 여정만 돌려주므로
// 직행 노선이 있으면 후보가 1~2개로 줄고(홍대입구→잠실: 2호선 하나), 수단별 탐색으로 대안을 채운다.
func (p *Planner) single(ctx context.Context, req PlanRequest) ([]otp.Itinerary, error) {
	base := otp.Request{Origin: coord(req.Origin), Destination: coord(req.Destination),
		WalkSpeed: req.WalkSpeed, BikeSpeed: req.BikeSpeed, Depart: req.Depart, First: DefaultFirst}
	base.OriginStop, base.DestStop = p.anchor(req.Origin), p.anchor(req.Destination)
	for _, v := range req.Via {
		base.Via = append(base.Via, coord(v))
		base.ViaStops = append(base.ViaStops, p.anchor(v)) // 경유 역도 역 ID 로(좌표면 역 구내 우회가 되살아난다)
	}
	variants := []otp.Modes{modesFor(ModeAny), transitOnlyModes("SUBWAY"), transitOnlyModes("BUS")}
	results := make([][]otp.Itinerary, len(variants))
	errs := make([]error, len(variants))
	var wg sync.WaitGroup
	for i, m := range variants {
		wg.Add(1)
		go func(i int, m otp.Modes) {
			defer wg.Done()
			r := base
			r.Modes = m
			results[i], errs[i] = p.OTP.Plan(ctx, r)
		}(i, m)
	}
	wg.Wait()
	var merged []otp.Itinerary
	for i := range variants {
		if errs[i] != nil && !errors.Is(errs[i], otp.ErrNoRoute) {
			return nil, errs[i]
		}
		merged = append(merged, results[i]...)
	}
	if len(merged) == 0 {
		if errs[0] != nil {
			return nil, errs[0]
		}
		return nil, otp.ErrNoRoute
	}
	return merged, nil
}

// transitOnlyModes 는 한 수단(SUBWAY/BUS)만 쓰는 대중교통 탐색. 접근·이탈은 도보.
func transitOnlyModes(mode string) otp.Modes {
	return otp.Modes{Transit: &otp.Transit{Access: []string{"WALK"}, Egress: []string{"WALK"}, Transfer: []string{"WALK"},
		Modes: []otp.TransitMode{{Mode: mode}}}, TransitOnly: true}
}

// segmented: 구간마다 top-BeamWidth 후보를 받아 앞 구간 도착시각을 다음 구간 출발시각으로 넘기며 잇는다.
// 결과는 "구간 제약을 순차 적용한 경로"이지 전역 최적이 아니다.
func (p *Planner) segmented(ctx context.Context, req PlanRequest) ([]otp.Itinerary, error) {
	pts := append(append([]Point{req.Origin}, req.Via...), req.Destination)
	beam := []partial{{}}
	for i := 0; i < len(pts)-1; i++ {
		// 빔의 후보마다 OTP 호출이 독립이라 병렬로 부른다(직렬이면 경유지 2개에 25~37초 실측).
		// 결과는 빔 순서대로 모아 prune 의 안정 정렬이 결정적이게 한다.
		results := make([][]partial, len(beam))
		errs := make([]error, len(beam))
		var wg sync.WaitGroup
		for bi, b := range beam {
			wg.Add(1)
			go func(bi int, b partial) {
				defer wg.Done()
				results[bi], errs[bi] = p.extend(ctx, req, pts, i, b)
			}(bi, b)
		}
		wg.Wait()
		var next []partial
		for bi := range beam {
			if errs[bi] != nil && !errors.Is(errs[bi], otp.ErrNoRoute) {
				return nil, errs[bi]
			}
			next = append(next, results[bi]...)
		}
		if len(next) == 0 {
			return nil, fmt.Errorf("%w: 구간 %d", otp.ErrNoRoute, i+1)
		}
		beam = prune(next)
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

// extend 는 부분 경로 b 를 구간 i(pts[i]→pts[i+1])로 한 번 더 잇는 후보들을 OTP 에서 받아 돌려준다.
func (p *Planner) extend(ctx context.Context, req PlanRequest, pts []Point, i int, b partial) ([]partial, error) {
	// First 는 빔 폭보다 넉넉히 받는다: OTP 가 같은 노선의 출발시각만 다른 복제를 앞에 몰아주므로
	// 빔 폭만큼만 받으면 중복 제거 뒤 후보가 1개로 줄어든다(실측).
	r := otp.Request{Origin: coord(pts[i]), Destination: coord(pts[i+1]), Modes: modesFor(req.Modes[i]),
		WalkSpeed: req.WalkSpeed, BikeSpeed: req.BikeSpeed, First: DefaultFirst}
	if m := req.Modes[i]; m == "" || m == ModeAny || m == ModeTransit { // 도보·자전거 전용 구간은 좌표로
		r.OriginStop, r.DestStop = p.anchor(pts[i]), p.anchor(pts[i+1])
	}
	if i == 0 {
		r.Depart = req.Depart
	} else {
		t := b.end
		r.Depart = &t
	}
	its, err := p.OTP.Plan(ctx, r)
	if err != nil {
		return nil, err
	}
	var out []partial
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
		out = append(out, np)
	}
	return out, nil
}

// partial 은 구간 분할 탐색에서 앞 구간까지 이어 붙인 부분 경로.
type partial struct {
	legs      []otp.Leg
	start     string
	end       time.Time
	endStr    string
	transfers int
	walk      float64
}

// prune 은 구간 후보를 빔 폭으로 줄인다. 같은 leg 서명은 가장 이른 도착 하나만 남기고,
// (도착시각·환승·도보) 세 축에서 지배되지 않는 후보를 먼저 채운 뒤 남는 자리는 도착시각순으로 채운다.
// 도착시각 하나로만 자르면 "조금 늦지만 환승이 적은" 후보가 구간 경계에서 전부 사라진다.
func prune(cands []partial) []partial {
	best := map[string]int{}
	var uniq []partial
	for _, c := range cands {
		sig := legSig(c.legs)
		if idx, ok := best[sig]; ok {
			if c.end.Before(uniq[idx].end) {
				uniq[idx] = c
			}
			continue
		}
		best[sig] = len(uniq)
		uniq = append(uniq, c)
	}
	sort.SliceStable(uniq, func(a, b int) bool { return uniq[a].end.Before(uniq[b].end) })
	var out []partial
	taken := make([]bool, len(uniq))
	for i, c := range uniq {
		dominated := false
		for _, k := range out { // k.end <= c.end 는 정렬로 보장
			if k.transfers <= c.transfers && k.walk <= c.walk {
				dominated = true
				break
			}
		}
		if !dominated && len(out) < BeamWidth {
			out = append(out, c)
			taken[i] = true
		}
	}
	for i, c := range uniq { // 남는 자리는 지배되더라도 도착이 이른 순으로(대안 다양성)
		if len(out) == BeamWidth {
			break
		}
		if !taken[i] {
			out = append(out, c)
		}
	}
	return out
}

// legSig 는 수단·노선·출발지·도착지 열로 만든 경로 서명. 출발시각만 다른 같은 경로를 묶는다.
func legSig(legs []otp.Leg) string {
	var sb strings.Builder
	for _, l := range legs {
		sb.WriteString(l.Mode + "|" + l.Route + "|" + l.FromName + "|" + l.ToName + ";")
	}
	return sb.String()
}

// dedupe 는 leg 서명이 같은 itinerary 중 가장 이른 출발만 남긴다.
// OTP 는 같은 경로를 출발시각만 다르게 여러 개 내기 때문이다(실측).
func dedupe(its []otp.Itinerary) []otp.Itinerary {
	seen := map[string]int{}
	var out []otp.Itinerary
	for _, it := range its {
		sig := legSig(it.Legs)
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
