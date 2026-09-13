// Package build 는 서울 버스 API 응답과 국가교통DB 도시철도를 합쳐 GTFS zip 을 만든다.
package build

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
	"github.com/SIDED00R/seoul-route/gtfs/internal/osm"
	"github.com/SIDED00R/seoul-route/gtfs/internal/seoulbus"
)

// 상수 출처·재보정 규칙
//   - FallbackSpeedKmh 18: sectSpd 결측(0) 구간에 쓰는 시내버스 평균속도 placeholder. 2026-09-12 표본(271번)에서
//     sectSpd 중앙값이 약 20km/h. Phase 3 이후 버스 실시간 위치로 구간별 실측치로 교체한다.
//   - 정차시간(dwellSec): sectSpd 는 주행속도라 정차가 빠져 있다. 2026-09-12(토) 21:10~21:20 버스 위치 API 로 14개 노선
//     차량을 20초 간격 추적한 구간 1,135표본에서 실측−모델 초과가 간선 +38초/구간(n=560), 지선 +30초(n=406),
//     마을 −5초(n=63), 광역 −19초(n=106, 고속도로 구간은 sectSpd 가 실제보다 느림). 저녁 한 시간대 표본이라
//     주간 첨두는 미측정. 재보정: 시간대별 추적 후 유형별 상수 갱신. 정류장별 첫차 통과시각(beginTm)은
//     같은 차량의 궤적이 아니라(마을버스 span 1,400분 실측) 검증에 못 쓴다.
//   - ServiceStart/End: 생성 시점 기준 유효 기간. 재생성 때마다 갱신.
const (
	FallbackSpeedKmh = 18.0
	DwellTrunkSec    = 38 // routeType 3 간선
	DwellBranchSec   = 30 // routeType 4 지선. 미측정 유형(순환·인천·경기·심야 등)도 정류장 간격이 비슷해 이 값을 쓴다
	ServiceStart     = "20260101"
	ServiceEnd       = "20301231"
	BusAgencyID      = "A_SEOULBUS"
	SubwayAgencyID   = "A1" // 파일럿 agency.txt 의 값 그대로
)

// BusRoute 는 fetch 결과 한 노선분이다.
type BusRoute struct {
	Route seoulbus.Route
	Stops []seoulbus.Stop
}

type RouteReport struct {
	RouteID, Name     string
	NStops            int
	TravelSec         int // 구간거리÷속도(+정차) 합, 왕복 전체
	Skipped           string
	DroppedDirections int // bbox 안 정류장이 2개 미만이라 뺀 방향 수
}

type Report struct {
	Routes       []RouteReport
	NBusRoutes   int
	NBusStops    int
	NSubwayTrips int
	NSubwayStops int
	NStations    int // 지하철 부모역(location_type=1)
	NPathways    int // 역 구내 통로(pathways.txt): 출입구↔승강장 + 승강장 간 환승
	NEntrances   int // 출입구(location_type=2) 행 수: OSM 출입구 + 승강장 좌표 출입구(폴백)
	// transfers.txt 행인데 부모역이 달라 통로가 안 생긴 수(행 단위, 양방향이면 2). 0 이 아니면 그 환승은 지상 도보로 계산된다
	NUnpairedTransfers        int
	NFarPairs                 int // transfers 값 없이 PathwayFallbackMaxM 을 넘어 통로를 안 만든 승강장 쌍(신촌 2호선↔경의중앙선)
	NRealEntranceStations     int // OSM 출입구가 붙은 부모역
	NFallbackEntranceStations int // OSM 출입구가 없어 승강장 좌표 출입구로 대신한 부모역(자식 2개 이상)
	NNoEntrancePlatforms      int // OSM 출입구 역인데 500m 안 출입구가 없어 통로 없이 고립되는 승강장. 0 이어야 한다
}

// Build 는 out 에 GTFS zip 을 쓴다. subway 는 nil 이면 버스만 쓴다. entrances 는 OSM 지하철 출입구(없으면 nil).
func Build(out string, buses []BusRoute, subway *ktdb.Subway, entrances []osm.Entrance) (*Report, error) {
	rep := &Report{}
	f, err := os.Create(out)
	if err != nil {
		return nil, err
	}
	closed := false
	zw := zip.NewWriter(f)
	// 조기 반환 경로의 핸들 정리. 성공 경로는 아래에서 명시적으로 닫고 오류를 돌려준다(중앙 디렉터리 기록은 Close 에서 일어난다).
	defer func() {
		if !closed {
			zw.Close()
			f.Close()
		}
	}()

	w := &writer{zw: zw}
	agencies := [][]string{{BusAgencyID, "서울시 버스", "https://bus.go.kr", "Asia/Seoul"}}
	if subway != nil {
		agencies = append(agencies, []string{SubwayAgencyID, "KTDB 도시철도 파일럿", "http://www.ktdb.go.kr/", "Asia/Seoul"})
	}
	w.table("agency.txt", []string{"agency_id", "agency_name", "agency_url", "agency_timezone"}, agencies)
	w.table("calendar.txt",
		[]string{"service_id", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday",
			"start_date", "end_date"},
		[][]string{{"ALL", "1", "1", "1", "1", "1", "1", "1", ServiceStart, ServiceEnd}})

	routes := [][]string{}
	trips := [][]string{}
	stopTimes := [][]string{}
	freqs := [][]string{}
	stops := map[string][]string{}
	for _, b := range buses {
		rr := busRoute(b, routes, &trips, &stopTimes, &freqs, stops)
		rep.Routes = append(rep.Routes, rr.report)
		if rr.report.Skipped == "" {
			routes = rr.routes
			rep.NBusRoutes++
		}
	}
	rep.NBusStops = len(stops)

	stopRows := make([][]string, 0, len(stops))
	for _, s := range stops {
		stopRows = append(stopRows, append(s, "", "")) // 버스 정류장은 부모역 없음
	}
	sort.Slice(stopRows, func(i, j int) bool { return stopRows[i][0] < stopRows[j][0] })

	if subway != nil {
		for _, r := range subway.Routes {
			routes = append(routes, []string{r["route_id"], SubwayAgencyID, r["route_short_name"], r["route_long_name"], "1"})
		}
		for _, t := range subway.Trips {
			trips = append(trips, []string{t["route_id"], "ALL", t["trip_id"], "", "0"})
		}
		for _, st := range subway.StopTimes {
			stopTimes = append(stopTimes,
				[]string{st["trip_id"], st["arrival_time"], st["departure_time"], st["stop_id"], st["stop_sequence"]})
		}
		parents, parentOf := stationGroups(subway.Stops)
		for _, s := range subway.Stops {
			stopRows = append(stopRows,
				[]string{s["stop_id"], s["stop_name"], s["stop_lat"], s["stop_lon"], "", parentOf[s["stop_id"]]})
		}
		for _, p := range parents {
			stopRows = append(stopRows, append(p, "1", ""))
		}
		rep.NStations = len(parents)
		tr := [][]string{}
		for _, t := range subway.Transfers {
			tr = append(tr, []string{t["from_stop_id"], t["to_stop_id"], t["transfer_type"], t["min_transfer_time"]})
		}
		w.table("transfers.txt", []string{"from_stop_id", "to_stop_id", "transfer_type", "min_transfer_time"}, tr)
		enRows, pw, st := stationPathways(subway.Stops, parents, parentOf, subway.Transfers, entrances)
		stopRows = append(stopRows, enRows...)
		w.table("pathways.txt",
			[]string{"pathway_id", "from_stop_id", "to_stop_id", "pathway_mode", "is_bidirectional", "traversal_time"}, pw)
		rep.NEntrances = len(enRows)
		rep.NPathways = len(pw)
		rep.NUnpairedTransfers = unpairedTransfers(subway.Transfers, parentOf)
		rep.NFarPairs = st.FarPairs
		rep.NRealEntranceStations = st.RealEntranceStns
		rep.NFallbackEntranceStations = st.FallbackStns
		rep.NNoEntrancePlatforms = st.NoEntrancePlatforms
		rep.NSubwayTrips = len(subway.Trips)
		rep.NSubwayStops = len(subway.Stops)
	}

	w.table("routes.txt", []string{"route_id", "agency_id", "route_short_name", "route_long_name", "route_type"}, routes)
	w.table("trips.txt", []string{"route_id", "service_id", "trip_id", "trip_headsign", "direction_id"}, trips)
	w.table("stop_times.txt", []string{"trip_id", "arrival_time", "departure_time", "stop_id", "stop_sequence"}, stopTimes)
	w.table("frequencies.txt", []string{"trip_id", "start_time", "end_time", "headway_secs", "exact_times"}, freqs)
	w.table("stops.txt", []string{"stop_id", "stop_name", "stop_lat", "stop_lon", "location_type", "parent_station"},
		stopRows)
	if w.err != nil {
		return nil, w.err
	}
	closed = true
	if err := zw.Close(); err != nil {
		f.Close()
		return nil, fmt.Errorf("zip 마무리 실패: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("zip 파일 닫기 실패: %w", err)
	}
	return rep, nil
}

type busResult struct {
	routes [][]string
	report RouteReport
}

// busRoute 는 노선 하나를 routes/trips/stop_times/frequencies 행으로 바꾼다.
// 회차 지점이 있으면 상행(`_T0`/`_LAST0`)·하행(`_T1`/`_LAST1`)으로 나누고, 없으면 접미사 없이 한 방향이다.
// 방향마다 trip 은 둘이다: (1) `_T` — 방향 첫 정류장 00:00:00 기준 상대시각 + frequencies(첫차~막차, exact_times=1).
// exact_times=0 이면 OTP 2.10 이 승차마다 배차간격 전체를 대기로 더한다(출발 위상과 무관, 실측 2026-09-12).
// exact_times=1 은 첫차부터 배차간격 격자로 출발하는 고정 시간표로 다뤄 대기가 위상에 따라 0~배차간격이 된다.
// GTFS 의 frequencies end_time 은 배타적이고 OTP 2.10 도 `< end` 로 비교하므로 막차 시각 자체의 출발은 생성되지 않는다.
// (2) `_LAST` — 막차 1회를 절대시각 stop_times 로 따로 둔다.
func busRoute(b BusRoute, routes [][]string, trips, stopTimes, freqs *[][]string, stops map[string][]string) busResult {
	r := b.Route
	rep := RouteReport{RouteID: r.ID, Name: r.Name, NStops: len(b.Stops)}
	term, _ := strconv.Atoi(strings.TrimSpace(r.TermMin))
	first, e1 := hhmmss(r.FirstBus)
	last, e2 := hhmmss(r.LastBus)
	switch {
	case len(b.Stops) < 2:
		rep.Skipped = "정류장 2개 미만"
	case term <= 0:
		rep.Skipped = "배차간격 없음"
	case e1 != nil || e2 != nil:
		rep.Skipped = "첫차/막차 시각 파싱 실패"
	case first == last:
		// API 는 첫차·막차를 모르면 둘 다 자정(…000000)으로 준다(실측 82노선, 전부 00:00:00). 24시간 운행이 아니다.
		rep.Skipped = "첫차·막차 시각 없음"
	}
	if rep.Skipped != "" {
		return busResult{routes, rep}
	}
	if last < first { // 막차가 자정 넘김
		last += 24 * 3600
	}
	routeID := "B_" + r.ID
	routes = append(routes, []string{routeID, BusAgencyID, r.Name, r.StartName + " ~ " + r.EndName, "3"})

	// 누적 소요(초)는 전 구간으로 계산한다. bbox 밖 정류장을 기록에서 빼도 그 구간의 주행시간은 이어져야 한다.
	times := make([]int, len(b.Stops))
	for i, s := range b.Stops {
		if i == 0 {
			continue
		}
		dist, _ := strconv.Atoi(strings.TrimSpace(s.SectDist))
		spd, _ := strconv.Atoi(strings.TrimSpace(s.SectSpd))
		kmh := float64(spd)
		if kmh <= 0 {
			kmh = FallbackSpeedKmh
		}
		times[i] = times[i-1] + int(float64(dist)/(kmh*1000/3600)+0.5) + dwellSec(r.Type)
	}
	rep.TravelSec = times[len(times)-1]

	// 회차 지점(transYn=Y)에서 상행(기점→회차)·하행(회차→종점) 두 trip 으로 나눈다. 한 trip 이면 OTP 가 회차지를
	// 지나 반대 방향까지 하차 없이 타는 경로를 만든다. 회차가 없거나 양 끝이면 한 방향(접미사 없음).
	k := -1
	for i, s := range b.Stops {
		if strings.TrimSpace(s.TransYn) == "Y" {
			k = i
			break
		}
	}
	type direction struct {
		from, to   int
		suffix, id string
		headsign   string
	}
	dirs := []direction{{0, len(b.Stops) - 1, "", "0", r.EndName}}
	if k > 0 && k < len(b.Stops)-1 {
		dirs = []direction{{0, k, "0", "0", b.Stops[k].Name}, {k, len(b.Stops) - 1, "1", "1", r.EndName}}
	}
	for _, d := range dirs {
		var inside []int // 서울 bbox 안의 정류장만 기록한다(밖은 도로망이 없어 OTP 에서 고립 정류장이 된다)
		for i := d.from; i <= d.to; i++ {
			if insideBBox(b.Stops[i]) {
				inside = append(inside, i)
			}
		}
		if len(inside) < 2 {
			rep.DroppedDirections++
			continue
		}
		tripID := routeID + "_T" + d.suffix
		lastTripID := routeID + "_LAST" + d.suffix
		*trips = append(*trips, []string{routeID, "ALL", tripID, d.headsign, d.id})
		*trips = append(*trips, []string{routeID, "ALL", lastTripID, d.headsign, d.id})
		// 기점 출발 후 이 방향의 첫 기록 정류장까지 걸리는 시간. bbox 클리핑으로 앞이 잘리면 회차지가 아니라
		// 첫 안쪽 정류장이 기준이다 — OTP 는 배차 trip 의 첫 stop_time 을 0 으로 정규화하므로(실측 441번: 누적
		// 53분 18초가 사라져 04:20 출발) frequencies 시작도 그만큼 늦춰야 절대시각 `_LAST` 와 맞는다.
		off := times[inside[0]]
		*freqs = append(*freqs, []string{tripID, fmtTime(first + off), fmtTime(last + off), strconv.Itoa(term * 60), "1"})
		for _, i := range inside {
			s := b.Stops[i]
			stopID := "BS_" + strings.TrimSpace(s.StationID)
			if _, ok := stops[stopID]; !ok {
				stops[stopID] = []string{stopID, s.Name, strings.TrimSpace(s.Lat), strings.TrimSpace(s.Lon)}
			}
			seq := strconv.Itoa(i + 1)
			rel := times[i] - off
			*stopTimes = append(*stopTimes, []string{tripID, fmtTime(rel), fmtTime(rel), stopID, seq})
			abs := last + times[i]
			*stopTimes = append(*stopTimes, []string{lastTripID, fmtTime(abs), fmtTime(abs), stopID, seq})
		}
	}
	if rep.DroppedDirections == len(dirs) { // 전 방향이 빠지면 노선 자체를 제외(routes.txt 고아 행·노선 수 과계 방지)
		rep.Skipped = "서울 bbox 안 정류장 2개 미만"
	}
	return busResult{routes, rep}
}

// 서울 OSM 추출 bbox(otp/extract_seoul.py, backend route.MinLon 등과 같은 값). 밖의 정류장은 도로망이 없어
// OTP 빌드에서 IsolatedStop 이 되므로 stop_times 에 넣지 않는다(정류장 18.8%·504노선 실측 2026-09-13).
const (
	BBoxMinLon, BBoxMinLat, BBoxMaxLon, BBoxMaxLat = 126.70, 37.38, 127.25, 37.75
)

func insideBBox(s seoulbus.Stop) bool {
	lon, e1 := strconv.ParseFloat(strings.TrimSpace(s.Lon), 64)
	lat, e2 := strconv.ParseFloat(strings.TrimSpace(s.Lat), 64)
	if e1 != nil || e2 != nil {
		return false
	}
	return lon >= BBoxMinLon && lon <= BBoxMaxLon && lat >= BBoxMinLat && lat <= BBoxMaxLat
}

// dwellSec 은 노선유형(API routeType)별 정류장 정차시간. 근거는 파일 머리 상수 주석.
func dwellSec(routeType string) int {
	switch strings.TrimSpace(routeType) {
	case "3":
		return DwellTrunkSec
	case "1", "2", "6": // 공항·마을·광역: 실측 초과가 0 이하이거나(마을·광역) 정류장이 드물다(공항)
		return 0
	default:
		return DwellBranchSec
	}
}

// hhmmss 는 "20260912041000" 에서 04:10:00 을 초로 돌려준다.
func hhmmss(s string) (int, error) {
	s = strings.TrimSpace(s)
	if len(s) != 14 {
		return 0, fmt.Errorf("시각 형식 아님: %q", s)
	}
	h, e1 := strconv.Atoi(s[8:10])
	m, e2 := strconv.Atoi(s[10:12])
	sec, e3 := strconv.Atoi(s[12:14])
	if e1 != nil || e2 != nil || e3 != nil {
		return 0, fmt.Errorf("시각 형식 아님: %q", s)
	}
	return h*3600 + m*60 + sec, nil
}

func fmtTime(sec int) string {
	return fmt.Sprintf("%02d:%02d:%02d", sec/3600, sec%3600/60, sec%60)
}

type writer struct {
	zw  *zip.Writer
	err error
}

func (w *writer) table(name string, header []string, rows [][]string) {
	if w.err != nil {
		return
	}
	var f io.Writer
	f, w.err = w.zw.Create(name)
	if w.err != nil {
		return
	}
	cw := csv.NewWriter(f)
	cw.UseCRLF = false
	if w.err = cw.Write(header); w.err != nil {
		return
	}
	if w.err = cw.WriteAll(rows); w.err != nil {
		return
	}
	w.err = cw.Error()
}
