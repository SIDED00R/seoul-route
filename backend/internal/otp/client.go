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

	"github.com/SIDED00R/seoul-route/backend/internal/fastexit"
)

type Coord struct {
	Lat float64 `json:"latitude"`
	Lon float64 `json:"longitude"`
}

// Modes 는 planConnection의 이동 수단 조건이다. BICYCLE_RENTAL은 WALK와 함께 사용한다.
type Modes struct {
	Direct      []string `json:"direct,omitempty"`
	Transit     *Transit `json:"transit,omitempty"`
	Only        bool     `json:"directOnly,omitempty"`
	TransitOnly bool     `json:"transitOnly,omitempty"` // 대중교통 없는 direct 후보를 억제한다
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
	// 여정에 맞는 stop 을 고르므로 역사 좌표가 엉뚱한 도로에 붙는 문제를 피한다.
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
	InStation  bool    `json:"in_station,omitempty"`   // 양끝이 같은 부모역의 정류장이고 역 출입구를 지나지 않는 도보(역 안 환승 통로)
	RentedBike bool    `json:"rented_bike,omitempty"`
	// 따릉이 대여 구간에서 빌릴 대여소의 남은 자전거 대수. 대여소를 못 찾거나 실시간 값이 오래됐으면 안 싣는다.
	BikesAvailable int    `json:"bikes_available,omitempty"`
	HasBikeCount   bool   `json:"has_bike_count,omitempty"` // 0대와 "모름" 을 구분한다
	TransitLeg     bool   `json:"transit_leg"`
	Polyline       string `json:"polyline,omitempty"` // Google encoded polyline
	// 시간표 기반 대중교통의 앞뒤 출발 시각. 버스는 HeadwaySec을 사용한다.
	PrevDepartures   []string `json:"prev_departures,omitempty"`
	NextDepartures   []string `json:"next_departures,omitempty"`
	HeadwaySec       int      `json:"headway_sec,omitempty"`           // 생성 GTFS frequencies 의 배차간격(버스)
	RealtimeArrivals []int    `json:"realtime_arrivals_sec,omitempty"` // 첫 탑승 정류장의 실시간 다음 차(초, 지금 기준)
	// 도보·따릉이 leg 가 지나는 신호 횡단보도 수와 그 기대 대기(초). Duration 에 이미 더해져 있다(route/crossing_hook.go).
	Crossings    int     `json:"crossings,omitempty"`
	CrossingWait float64 `json:"crossing_wait_sec,omitempty"`
	Headsign     string  `json:"headsign,omitempty"` // 탑승 차량이 정류장에 내거는 행선지(대중교통 leg)
	// 노선 색(생성 GTFS routes.txt, # 없는 6자리 16진수). 앱이 구간 칩·경로선을 이 색으로 칠한다(route/plan.go 가 붙인다).
	Color     string `json:"color,omitempty"`
	TextColor string `json:"text_color,omitempty"`
	Steps     []Step `json:"steps,omitempty"` // 도보·자전거 leg 의 안내 단계
	Stops     []Stop `json:"stops,omitempty"` // 대중교통 leg 의 중간 정차(탑승·하차 제외)
	// 지하철 하차역에서 설비(계단·에스컬레이터·엘리베이터)가 있는 칸-문. 자료가 있는 역(1~8호선)만 채운다.
	FastExit []fastexit.Facility `json:"fast_exit,omitempty"`
}

// Step 은 도보·자전거 leg 의 안내 단계. Dir 은 이 단계 시작점에서의 회전(OTP relativeDirection),
// Distance 는 그 뒤로 가는 거리. Street 는 OTP 가 bogusName 으로 표시한 생성 이름(path·sidewalk 등)이면 비운다.
type Step struct {
	Dir      string  `json:"dir"`
	Abs      string  `json:"abs,omitempty"`
	Street   string  `json:"street,omitempty"`
	Distance float64 `json:"distance_m"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Exit     string  `json:"exit,omitempty"`     // 회전교차로 출구 번호
	Entrance string  `json:"entrance,omitempty"` // 역 출입구 이름(ENTER_STATION·EXIT_STATION), 예 "강남 8번 출구"
}

// Stop 은 대중교통 leg 의 중간 정차. OffsetSec 은 leg 출발(Start) 기준 도착까지의 초 — 상대값이라
// 실시간 보정이 Start·End 를 함께 옮겨도 그대로 쓴다(realtime/corrector.go).
type Stop struct {
	Name      string  `json:"name"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	StopID    string  `json:"stop_id,omitempty"`
	OffsetSec int     `json:"offset_sec,omitempty"`
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
	// 첫 차가 시간표보다 일러도 뒤에 대중교통 탑승이 더 있으면 도착은 그대로라 0 이다. 첫 차가 늦을 때 지하철끼리 환승은
	// 여유 안이면 0, 넘으면 다음 열차 기준이라 지연보다 클 수 있다(realtime/transfer_connect.go).
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
             from { name lat lon stop { gtfsId parentStation { gtfsId } } }
             to { name lat lon stop { gtfsId parentStation { gtfsId } } } route { shortName gtfsId }
             headsign intermediatePlaces { name lat lon stop { gtfsId } arrival { scheduledTime } }
             legGeometry { points }
             steps { relativeDirection absoluteDirection streetName bogusName distance lat lon exit
                     feature { ... on Entrance { name } } }
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
	prefs := map[string]any{"bicycle": bicyclePrefs(r.BikeSpeed)}
	if r.WalkSpeed > 0 {
		prefs["walk"] = map[string]any{"speed": r.WalkSpeed}
	}
	vars["preferences"] = map[string]any{"street": prefs}
	if r.Depart != nil {
		vars["dateTime"] = map[string]any{"earliestDeparture": r.Depart.Format(time.RFC3339)}
	}
	var out response
	if err := c.query(ctx, map[string]any{"query": query, "variables": vars}, 16<<20, &out); err != nil {
		return nil, err
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

// BikeWalkReluctance 2.0: 자전거를 끌고 걷는 시간에 곱하는 비용. OTP 기본 5.0 에서는 끌어야 지나가는 구간
// (bicycle=dismount 인 한강 다리 보도 등)을 피해 크게 돌아간다. docs/bicycle-routing.md
const BikeWalkReluctance = 2.0

// bicyclePrefs 는 자전거 선호. 최적화 기준은 소요시간(SHORTEST_DURATION)이다 — OTP 기본 SAFE_STREETS 는
// 자전거길을 우대해 더 오래 걸리는 경로를 고른다. speed 는 평지 최대속도이며 0 이면 OTP 기본값을 쓴다.
func bicyclePrefs(speed float64) map[string]any {
	p := map[string]any{
		"optimization": map[string]any{"type": "SHORTEST_DURATION"},
		"walk":         map[string]any{"cost": map[string]any{"reluctance": BikeWalkReluctance}},
	}
	if speed > 0 {
		p["speed"] = speed
	}
	return p
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
	var out struct {
		gqlErrors
		Data struct {
			Stations []struct {
				GtfsID   string `json:"gtfsId"`
				Name     string
				Lat, Lon float64
			}
		} `json:"data"`
	}
	if err := c.query(ctx, map[string]any{"query": stationsQuery}, 4<<20, &out); err != nil {
		return nil, err
	}
	sts := make([]Station, 0, len(out.Data.Stations))
	for _, s := range out.Data.Stations {
		sts = append(sts, Station{ID: s.GtfsID, Name: s.Name, Lat: s.Lat, Lon: s.Lon})
	}
	return sts, nil
}

// gqlErrors 는 GraphQL 응답의 errors 부분. 응답 구조체마다 묻어 둔다.
type gqlErrors struct {
	Errors []struct{ Message string } `json:"errors"`
}

func (g gqlErrors) err() error {
	if len(g.Errors) > 0 {
		return fmt.Errorf("otp: %s", g.Errors[0].Message)
	}
	return nil
}

// query 는 GraphQL 한 번을 보내고 응답을 out 에 담는다. limit 은 읽을 본문 상한(바이트).
func (c *Client) query(ctx context.Context, body map[string]any, limit int64, out interface{ err() error }) error {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/otp/gtfs/v1", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("otp: %w", err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, limit)).Decode(out); err != nil {
		return fmt.Errorf("otp: 응답 파싱 실패 (HTTP %d)", resp.StatusCode)
	}
	return out.err()
}

type response struct {
	gqlErrors
	Data struct {
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
			Stop     *legStop
		}
		To struct {
			Name     string
			Lat, Lon float64
			Stop     *legStop
		}
		Route *struct {
			ShortName string
			GtfsID    string `json:"gtfsId"`
		}
		Headsign           string
		IntermediatePlaces []struct {
			Name     string
			Lat, Lon float64
			Stop     *struct {
				GtfsID string `json:"gtfsId"`
			}
			Arrival *struct{ ScheduledTime string }
		}
		LegGeometry  *struct{ Points string }
		Steps        []legStep
		PreviousLegs []legTime
		NextLegs     []legTime
	}
}

// legStep 은 OTP step 원문. Feature 는 union(Entrance·StairsUse 등)이라 Entrance 가 아니면 Name 이 빈다.
type legStep struct {
	RelativeDirection string
	AbsoluteDirection string
	StreetName        string
	BogusName         bool
	Distance          float64
	Lat, Lon          float64
	Exit              string
	Feature           *struct{ Name string }
}

// legStop 은 leg 양끝의 정류장(없으면 nil — 좌표 출발·도착). 생성 GTFS 의 지하철 승강장은 부모역(ST_…)을 가진다.
type legStop struct {
	GtfsID        string `json:"gtfsId"`
	ParentStation *struct {
		GtfsID string `json:"gtfsId"`
	} `json:"parentStation"`
}

// sameParentStation 은 두 정류장이 모두 있고 같은 부모역에 속하는지.
func sameParentStation(a, b *legStop) bool {
	return a != nil && b != nil && a.ParentStation != nil && b.ParentStation != nil &&
		a.ParentStation.GtfsID != "" && a.ParentStation.GtfsID == b.ParentStation.GtfsID
}

// leavesStation 은 도보 steps 에 역 출입구로 나가거나 들어오는 지점(EXIT_STATION·ENTER_STATION)이 있는지.
func leavesStation(steps []legStep) bool {
	for _, s := range steps {
		if s.RelativeDirection == "EXIT_STATION" || s.RelativeDirection == "ENTER_STATION" {
			return true
		}
	}
	return false
}

type legTime struct {
	Start    struct{ ScheduledTime string }
	Duration float64
}

// nearbyDepartures 는 기준 시각 ±3시간 안에서 소요시간이 비슷한 앞뒤 차만 돌려준다.
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
		leg.InStation = l.Mode == "WALK" && sameParentStation(l.From.Stop, l.To.Stop) && !leavesStation(l.Steps)
		if len(l.IntermediatePlaces) > 0 {
			leg.NextStop = l.IntermediatePlaces[0].Name
		} else if l.TransitLeg {
			leg.NextStop = l.To.Name
		}
		if l.LegGeometry != nil {
			leg.Polyline = l.LegGeometry.Points
		}
		leg.Headsign = l.Headsign
		for _, s := range l.Steps {
			step := Step{Dir: s.RelativeDirection, Abs: s.AbsoluteDirection, Distance: s.Distance,
				Lat: s.Lat, Lon: s.Lon, Exit: s.Exit}
			if !s.BogusName { // OTP 가 이름 없는 길에 붙이는 생성 이름(path·sidewalk·pathway)은 안내에 쓰지 않는다
				step.Street = s.StreetName
			}
			if s.Feature != nil {
				step.Entrance = s.Feature.Name
			}
			leg.Steps = append(leg.Steps, step)
		}
		legStart, startErr := time.Parse(time.RFC3339, leg.Start)
		for _, p := range l.IntermediatePlaces {
			stop := Stop{Name: p.Name, Lat: p.Lat, Lon: p.Lon}
			if p.Stop != nil {
				stop.StopID = p.Stop.GtfsID
			}
			if startErr == nil && p.Arrival != nil {
				if t, err := time.Parse(time.RFC3339, p.Arrival.ScheduledTime); err == nil {
					stop.OffsetSec = int(t.Sub(legStart).Seconds())
				}
			}
			leg.Stops = append(leg.Stops, stop)
		}
		it.Legs = append(it.Legs, leg)
	}
	return it
}
