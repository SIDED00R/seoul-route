package route

import (
	"fmt"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

func validate(req *PlanRequest) error {
	points := append([]Point{req.Origin, req.Destination}, req.Via...)
	for _, point := range points {
		if point.Lon < MinLon || point.Lon > MaxLon || point.Lat < MinLat || point.Lat > MaxLat {
			return fmt.Errorf("%w: 좌표가 서울 범위 밖 (%.4f, %.4f)", ErrBadRequest, point.Lat, point.Lon)
		}
	}
	if len(req.Via) > 5 {
		return fmt.Errorf("%w: 경유지는 5개까지", ErrBadRequest)
	}
	wantModes := len(req.Via) + 1
	if len(req.Modes) == 0 {
		req.Modes = make([]SegmentMode, wantModes)
	}
	if len(req.Modes) != wantModes {
		return fmt.Errorf("%w: segment_modes 길이는 경유지 수+1 (%d)", ErrBadRequest, wantModes)
	}
	for i, mode := range req.Modes {
		switch mode {
		case "", ModeAny:
			req.Modes[i] = ModeAny
		case ModeWalk, ModeBike, ModeTransit:
		default:
			return fmt.Errorf("%w: segment_modes 값 %q", ErrBadRequest, mode)
		}
	}
	return nil
}

func modesFor(mode SegmentMode) otp.Modes {
	switch mode {
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

func coord(point Point) otp.Coord { return otp.Coord{Lat: point.Lat, Lon: point.Lon} }

func satisfies(mode SegmentMode, itinerary otp.Itinerary) bool {
	for _, leg := range itinerary.Legs {
		switch {
		case mode == ModeBike && (leg.RentedBike || leg.Mode == "BICYCLE"):
			return true
		case mode == ModeTransit && leg.TransitLeg:
			return true
		}
	}
	return mode != ModeBike && mode != ModeTransit
}

func anchorsSegment(mode SegmentMode) bool {
	return mode == "" || mode == ModeAny || mode == ModeTransit
}
