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
	"strings"
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
	Access   []string      `json:"access"`
	Egress   []string      `json:"egress"`
	Transfer []string      `json:"transfer"`
	Modes    []TransitMode `json:"transit,omitempty"` // 비면 전 수단. 지하철만·버스만 탐색에 쓴다
}

type TransitMode struct {
	Mode string `json:"mode"` // SUBWAY, BUS, RAIL ...
}

type Request struct {
	Origin, Destination Coord
	// OriginStop/DestStop 이 있으면 좌표 대신 역(gtfsId, 예 "seoul:ST_서울")으로 요청한다. OTP 가 역 안에서
	// 여정에 맞는 stop 을 고르므로 역사 좌표가 엉뚱한 도로에 붙는 문제를 피한다(이슈 #12).
	OriginStop, DestStop string
	Via                  []Coord
	ViaStops             []string // Via 와 같은 길이. 비어 있지 않은 항목은 좌표 대신 그 역 ID 로 경유한다
	Modes                Modes
	WalkSpeed            float64    // m/s, 0 이면 OTP 기본
	BikeSpeed            float64    // m/s
	Depart               *time.Time // nil = 지금 출발(OTP 가 대여소 실시간 잔여대수를 반영하는 유일한 경우)
	First                int
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
	RouteID    string  `json:"route_id,omitempty"`     // gtfsId, 예 "seoul:B_100100063" / "seoul:RR_…"
	FromStopID string  `json:"from_stop_id,omitempty"` // 탑승 정류장 gtfsId, 예 "seoul:BS_123000354"
	NextStop   string  `json:"next_stop,omitempty"`    // 탑승 후 첫 정차역 이름(지하철 방면 판별용)
	RentedBike bool    `json:"rented_bike,omitempty"`
	TransitLeg bool    `json:"transit_leg"`
	Polyline   string  `json:"polyline,omitempty"` // Google encoded polyline
	// 앞뒤 차: 같은 탑승·하차 정류장의 이전/다음 출발(RFC3339). 시간표 기반(지하철)에서만 채운다 — 배차간격 기반
	// 버스는 OTP 가 막차 trip 만 돌려줘(실측) 비워 두고 HeadwaySec 을 쓴다. leg 출발 ±3시간 밖과, 소요시간이
	// 현재 leg 의 0.5~2배 밖인 것(순환선 반대 방향 열차)은 버린다.
	PrevDepartures   []string `json:"prev_departures,omitempty"`
	NextDepartures   []string `json:"next_departures,omitempty"`
	HeadwaySec       int      `json:"headway_sec,omitempty"`           // 생성 GTFS frequencies 의 배차간격(버스)
	RealtimeArrivals []int    `json:"realtime_arrivals_sec,omitempty"` // 첫 탑승 정류장의 실시간 다음 차(초, 지금 기준)
	// 도보·따릉이 leg 가 지나는 신호 횡단보도 수와 그 기대 대기(초). Duration 에 이미 더해져 있다(route/crossing_hook.go).
	Crossings    int     `json:"crossings,omitempty"`
	CrossingWait float64 `json:"crossing_wait_sec,omitempty"`
}

type Itinerary struct {
	Start     string  `json:"start"`
	End       string  `json:"end"`
	Duration  float64 `json:"duration_sec"`
	Transfers int     `json:"transfers"`
	WalkM     float64 `json:"walk_distance_m"`
	Legs      []Leg   `json:"legs"`
	// DepartIn: 요청 시각부터 이 여정의 출발(Start)까지 기다리는 초. "지금 출발" 요청에서만 채우고,
	// 순위와 앱의 총 소요 표시는 Duration+DepartIn 을 쓴다(OTP 의 Duration 은 Start 부터라 출발 전 대기가 빠져 있다).
	DepartIn float64 `json:"depart_in_sec,omitempty"`
	// Realtime: 첫 탑승을 실시간 도착정보로 보정했을 때 true. RealtimeDelta 는 보정 뒤 도착(End) 이동량(초, 음수 가능).
	// 첫 차가 시간표보다 일러도 뒤에 대중교통 탑승이 더 있으면 도착은 그대로라 0 이다.
	Realtime      bool    `json:"realtime,omitempty"`
	RealtimeDelta float64 `json:"realtime_delta_sec,omitempty"`
	// CrossingWait: 도보·따릉이 구간의 신호 횡단보도 기대 대기 합(초, Duration 에 포함). Replanned: 그 대기로 다음 탑승을
	// 놓치게 돼 그 지점부터 다시 탐색해 뒤 구간을 갈아 끼웠다.
	CrossingWait float64 `json:"crossing_wait_sec,omitempty"`
	Replanned    bool    `json:"replanned,omitempty"`
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
             from { name lat lon stop { gtfsId } } to { name lat lon } route { shortName gtfsId }
             intermediateStops { name } legGeometry { points }
             previousLegs(numberOfLegs: 5) { start { scheduledTime } duration }
             nextLegs(numberOfLegs: 7) { start { scheduledTime } duration } }
    } }
  }
}`

// ErrNoRoute 는 OTP 가 routingErrors 만 돌려주고 경로가 없을 때.
var ErrNoRoute = errors.New("경로 없음")

// Plan 은 planConnection 을 호출해 itinerary 목록을 돌려준다. 경로가 없으면 ErrNoRoute(원인 포함).
func (c *Client) Plan(ctx context.Context, r Request) ([]Itinerary, error) {
	vars := map[string]any{
		"origin":      loc(r.Origin, r.OriginStop),
		"destination": loc(r.Destination, r.DestStop),
		"modes":       r.Modes, // 생략하면 via 처리에서 OTP NPE(실측) — 항상 보낸다
		"first":       r.First,
	}
	if len(r.Via) > 0 {
		via := make([]map[string]any, 0, len(r.Via))
		for i, v := range r.Via {
			visit := map[string]any{"coordinate": v, "minimumWaitTime": "PT0S"}
			if i < len(r.ViaStops) && r.ViaStops[i] != "" {
				visit = map[string]any{"stopLocationIds": []string{r.ViaStops[i]}, "minimumWaitTime": "PT0S"}
			}
			via = append(via, map[string]any{"visit": visit})
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

func loc(c Coord, stop string) map[string]any {
	if stop != "" {
		return map[string]any{"location": map[string]any{"stopLocation": map[string]any{"stopLocationId": stop}}}
	}
	return map[string]any{"location": map[string]any{"coordinate": c}}
}

// Station 은 GTFS 부모역(location_type=1). Name 은 괄호 없는 기준명("서울").
type Station struct {
	ID   string // gtfsId, 예 "seoul:ST_서울"
	Name string
	Lat  float64
	Lon  float64
}

const stationsQuery = `{ stations { gtfsId name lat lon } }`

// Stations 는 그래프의 부모역 전부를 돌려준다. 기동 시 한 번 불러 앵커링에 쓴다.
func (c *Client) Stations(ctx context.Context) ([]Station, error) {
	body, _ := json.Marshal(map[string]any{"query": stationsQuery})
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
	var out struct {
		Errors []struct{ Message string } `json:"errors"`
		Data   struct {
			Stations []struct {
				GtfsID   string `json:"gtfsId"`
				Name     string
				Lat, Lon float64
			}
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("otp: stations 응답 파싱 실패 (HTTP %d)", resp.StatusCode)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("otp: %s", out.Errors[0].Message)
	}
	sts := make([]Station, 0, len(out.Data.Stations))
	for _, s := range out.Data.Stations {
		sts = append(sts, Station{ID: s.GtfsID, Name: s.Name, Lat: s.Lat, Lon: s.Lon})
	}
	return sts, nil
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
		From       struct {
			Name     string
			Lat, Lon float64
			Stop     *struct {
				GtfsID string `json:"gtfsId"`
			}
		}
		To struct {
			Name     string
			Lat, Lon float64
		}
		Route *struct {
			ShortName string
			GtfsID    string `json:"gtfsId"`
		}
		IntermediateStops []struct{ Name string }
		LegGeometry       *struct{ Points string }
		PreviousLegs      []legTime
		NextLegs          []legTime
	}
}

type legTime struct {
	Start    struct{ ScheduledTime string }
	Duration float64
}

// nearbyDepartures 는 앞뒤 차 중 기준 시각 ±3시간 안이고 소요시간이 현재 leg 의 0.5~2배인 것만 돌려준다.
// ±3시간은 배차 기반 trip 의 막차 sentinel 제거. 소요시간 조건은 순환선(2호선)에서 같은 두 역을 반대 방향으로
// 한 바퀴 돌아 잇는 열차(9분 구간에 81분짜리, 실측)를 빼기 위한 것이다. 노선 ID 로 거르면 1호선처럼
// 계열 노선(1U·7U·2U)이 같은 구간을 같은 시간에 달리는 정상 항목까지 빠진다(실측).
func nearbyDepartures(legs []legTime, ref string, refDuration float64) []string {
	base, err := time.Parse(time.RFC3339, ref)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range legs {
		t, err := time.Parse(time.RFC3339, l.Start.ScheduledTime)
		if err != nil {
			continue
		}
		if refDuration > 0 && (l.Duration < refDuration*0.5 || l.Duration > refDuration*2) {
			continue
		}
		if d := t.Sub(base); d > -3*time.Hour && d < 3*time.Hour && d != 0 {
			out = append(out, l.Start.ScheduledTime)
		}
	}
	return out
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
			leg.RouteID = l.Route.GtfsID
		}
		if l.TransitLeg && !strings.HasPrefix(leg.RouteID, "seoul:B_") { // 버스(배차 기반)는 HeadwaySec 으로
			leg.PrevDepartures = nearbyDepartures(l.PreviousLegs, leg.Start, leg.Duration)
			leg.NextDepartures = nearbyDepartures(l.NextLegs, leg.Start, leg.Duration)
		}
		if l.From.Stop != nil {
			leg.FromStopID = l.From.Stop.GtfsID
		}
		if len(l.IntermediateStops) > 0 {
			leg.NextStop = l.IntermediateStops[0].Name
		} else if l.TransitLeg {
			leg.NextStop = l.To.Name
		}
		if l.LegGeometry != nil {
			leg.Polyline = l.LegGeometry.Points
		}
		it.Legs = append(it.Legs, leg)
	}
	return it
}
