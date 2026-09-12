// Package otp 는 OpenTripPlanner 2.10 GTFS GraphQL API 의 planConnection 클라이언트다.
package otp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Coord struct {
	Lat float64 `json:"latitude"`
	Lon float64 `json:"longitude"`
}

// Modes 는 planConnection 의 modes 인자. Direct 만 있으면 도보·자전거 전용 탐색(directOnly).
// BICYCLE_RENTAL 은 항상 WALK 와 함께 넣어야 한다(OTP 2.10 BadRequest 실측).
type Modes struct {
	Direct      []string `json:"direct,omitempty"`
	Transit     *Transit `json:"transit,omitempty"`
	Only        bool     `json:"directOnly,omitempty"`
	TransitOnly bool     `json:"transitOnly,omitempty"` // 대중교통 없는 direct 후보를 억제한다(실측)
}

type Transit struct {
	Access   []string `json:"access"`
	Egress   []string `json:"egress"`
	Transfer []string `json:"transfer"`
}

type Request struct {
	Origin, Destination Coord
	Via                 []Coord
	Modes               Modes
	WalkSpeed           float64    // m/s, 0 이면 OTP 기본
	BikeSpeed           float64    // m/s
	Depart              *time.Time // nil = 지금 출발(OTP 가 대여소 실시간 잔여대수를 반영하는 유일한 경우)
	First               int
}

type Leg struct {
	Mode       string  `json:"mode"`
	Start      string  `json:"start"` // RFC3339
	End        string  `json:"end"`
	Duration   float64 `json:"duration_sec"`
	Distance   float64 `json:"distance_m"`
	FromName   string  `json:"from_name"`
	FromLat    float64 `json:"from_lat"`
	FromLon    float64 `json:"from_lon"`
	ToName     string  `json:"to_name"`
	ToLat      float64 `json:"to_lat"`
	ToLon      float64 `json:"to_lon"`
	Route      string  `json:"route,omitempty"`
	RentedBike bool    `json:"rented_bike,omitempty"`
	TransitLeg bool    `json:"transit_leg"`
	Polyline   string  `json:"polyline,omitempty"` // Google encoded polyline
}

type Itinerary struct {
	Start     string  `json:"start"`
	End       string  `json:"end"`
	Duration  float64 `json:"duration_sec"`
	Transfers int     `json:"transfers"`
	WalkM     float64 `json:"walk_distance_m"`
	Legs      []Leg   `json:"legs"`
}

type Client struct {
	URL  string // 예: http://localhost:8080
	HTTP *http.Client
}

const query = `
query Plan($origin: PlanLabeledLocationInput!, $destination: PlanLabeledLocationInput!,
           $via: [PlanViaLocationInput!], $modes: PlanModesInput, $preferences: PlanPreferencesInput,
           $dateTime: PlanDateTimeInput, $first: Int) {
  planConnection(origin: $origin, destination: $destination, via: $via, modes: $modes,
                 preferences: $preferences, dateTime: $dateTime, first: $first) {
    routingErrors { code description }
    edges { node {
      start end duration numberOfTransfers walkDistance
      legs { mode duration distance rentedBike transitLeg
             start { scheduledTime } end { scheduledTime }
             from { name lat lon } to { name lat lon } route { shortName } legGeometry { points } }
    } }
  }
}`

// ErrNoRoute 는 OTP 가 routingErrors 만 돌려주고 경로가 없을 때.
var ErrNoRoute = errors.New("경로 없음")

// Plan 은 planConnection 을 호출해 itinerary 목록을 돌려준다. 경로가 없으면 ErrNoRoute(원인 포함).
func (c *Client) Plan(ctx context.Context, r Request) ([]Itinerary, error) {
	vars := map[string]any{
		"origin":      loc(r.Origin),
		"destination": loc(r.Destination),
		"modes":       r.Modes, // 생략하면 via 처리에서 OTP NPE(실측) — 항상 보낸다
		"first":       r.First,
	}
	if len(r.Via) > 0 {
		via := make([]map[string]any, 0, len(r.Via))
		for _, v := range r.Via {
			via = append(via, map[string]any{"visit": map[string]any{"coordinate": v, "minimumWaitTime": "PT0S"}})
		}
		vars["via"] = via
	}
	prefs := map[string]any{}
	if r.WalkSpeed > 0 {
		prefs["walk"] = map[string]any{"speed": r.WalkSpeed}
	}
	if r.BikeSpeed > 0 {
		prefs["bicycle"] = map[string]any{"speed": r.BikeSpeed}
	}
	if len(prefs) > 0 {
		vars["preferences"] = map[string]any{"street": prefs}
	}
	if r.Depart != nil {
		vars["dateTime"] = map[string]any{"earliestDeparture": r.Depart.Format(time.RFC3339)}
	}
	body, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/otp/gtfs/v1", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("otp: %w", err)
	}
	defer resp.Body.Close()
	var out response
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("otp: 응답 파싱 실패 (HTTP %d)", resp.StatusCode)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("otp: %s", out.Errors[0].Message)
	}
	pc := out.Data.PlanConnection
	if len(pc.Edges) == 0 {
		if len(pc.RoutingErrors) > 0 {
			return nil, fmt.Errorf("%w: %s", ErrNoRoute, pc.RoutingErrors[0].Code)
		}
		return nil, ErrNoRoute
	}
	its := make([]Itinerary, 0, len(pc.Edges))
	for _, e := range pc.Edges {
		its = append(its, e.Node.itinerary())
	}
	return its, nil
}

func loc(c Coord) map[string]any {
	return map[string]any{"location": map[string]any{"coordinate": c}}
}

type response struct {
	Errors []struct{ Message string } `json:"errors"`
	Data   struct {
		PlanConnection struct {
			RoutingErrors []struct{ Code, Description string } `json:"routingErrors"`
			Edges         []struct{ Node node }                `json:"edges"`
		} `json:"planConnection"`
	} `json:"data"`
}

type node struct {
	Start, End        string
	Duration          float64
	NumberOfTransfers int
	WalkDistance      float64
	Legs              []struct {
		Mode       string
		Duration   float64
		Distance   float64
		RentedBike bool
		TransitLeg bool
		Start, End struct{ ScheduledTime string }
		From, To   struct {
			Name     string
			Lat, Lon float64
		}
		Route       *struct{ ShortName string }
		LegGeometry *struct{ Points string }
	}
}

func (n node) itinerary() Itinerary {
	it := Itinerary{Start: n.Start, End: n.End, Duration: n.Duration, Transfers: n.NumberOfTransfers,
		WalkM: n.WalkDistance}
	for _, l := range n.Legs {
		leg := Leg{Mode: l.Mode, Start: l.Start.ScheduledTime, End: l.End.ScheduledTime, Duration: l.Duration,
			Distance: l.Distance, FromName: l.From.Name, FromLat: l.From.Lat, FromLon: l.From.Lon,
			ToName: l.To.Name, ToLat: l.To.Lat, ToLon: l.To.Lon, RentedBike: l.RentedBike, TransitLeg: l.TransitLeg}
		if l.Route != nil {
			leg.Route = l.Route.ShortName
		}
		if l.LegGeometry != nil {
			leg.Polyline = l.LegGeometry.Points
		}
		it.Legs = append(it.Legs, leg)
	}
	return it
}
