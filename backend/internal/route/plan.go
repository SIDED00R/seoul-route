// Package route 는 앱의 요청을 OTP 호출로 바꾸고 후보를 결합·보정한다.
package route

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/fastexit"
	"github.com/SIDED00R/seoul-route/backend/internal/otp"
	"github.com/SIDED00R/seoul-route/backend/internal/routestyle"
)

// 학습한 이동 속도를 OTP 라우팅 속도로 변환하는 계수다.
const (
	MinLon, MinLat, MaxLon, MaxLat = 126.70, 37.38, 127.25, 37.75
	DefaultWalk                    = 1.23
	DefaultBike                    = 5.0
	walkEffective                  = 0.973
	bikeEffective                  = 0.89
	BeamWidth                      = 3
	DefaultFirst                   = 10
)

func WalkOTPSpeed(avg float64) float64 { return avg / walkEffective }
func BikeOTPSpeed(avg float64) float64 { return avg / bikeEffective }

type SegmentMode string

const (
	ModeAny     SegmentMode = "any"
	ModeWalk    SegmentMode = "walk"
	ModeBike    SegmentMode = "bike"
	ModeTransit SegmentMode = "transit"
)

type Point struct {
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Name string  `json:"name,omitempty"`
}

type PlanRequest struct {
	Origin      Point         `json:"origin"`
	Destination Point         `json:"destination"`
	Via         []Point       `json:"via"`
	Modes       []SegmentMode `json:"segment_modes"`
	Depart      *time.Time    `json:"depart"`
	WalkSpeed   float64       `json:"-"`
	BikeSpeed   float64       `json:"-"`
}

type Planner struct {
	OTP interface {
		Plan(context.Context, otp.Request) ([]otp.Itinerary, error)
	}
	Realtime interface {
		Adjust(context.Context, []otp.Itinerary) []otp.Itinerary
	}
	Crossings   Crossings
	CrossingSec float64
	Now         func() time.Time
	Headways    map[string]int
	RouteStyles map[string]routestyle.Style
	Bikes       BikeStations
	FastExits   *fastexit.Index
	mu          sync.RWMutex
	stations    []otp.Station
}

func (p *Planner) SetStations(stations []otp.Station) {
	p.mu.Lock()
	p.stations = stations
	p.mu.Unlock()
}

var ErrBadRequest = errors.New("bad request")

// Plan 은 요청 형태에 맞는 탐색을 실행한 뒤 시간·실시간·표시 메타데이터를 적용한다.
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
	for _, mode := range req.Modes {
		if mode != ModeAny {
			locked = true
			break
		}
	}

	var (
		its []otp.Itinerary
		err error
	)
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
	now := p.now()
	destAnchored := anchorsSegment(req.Modes[len(req.Modes)-1]) && p.anchor(req.Destination) != ""
	applyStationSlack(its, anchorsSegment(req.Modes[0]) && p.anchor(req.Origin) != "", destAnchored)
	setDepartIn(its, req.Depart, now)
	if p.Crossings != nil {
		its = p.applyCrossings(ctx, its, req, p.CrossingSec, destAnchored)
	}
	if p.Realtime != nil && req.Depart == nil {
		sortByScore(its)
		its = p.Realtime.Adjust(ctx, its)
		setDepartIn(its, nil, now)
	}

	its = rank(its)
	p.annotateTransitMetadata(its)
	p.annotateBikeStations(its)
	p.annotateFastExits(its)
	return its, nil
}

func (p *Planner) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}
