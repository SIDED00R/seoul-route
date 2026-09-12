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
	"github.com/SIDED00R/seoul-route/gtfs/internal/seoulbus"
)

// 상수 출처·재보정 규칙
//   - FallbackSpeedKmh 18: sectSpd 결측(0) 구간에 쓰는 시내버스 평균속도 placeholder. 2026-09-12 표본(271번)에서
//     sectSpd 중앙값이 약 20km/h. Phase 3 이후 버스 실시간 위치로 구간별 실측치로 교체한다.
//   - DwellSec 0: 정차시간은 sectSpd 가 구간 평균속도라 이미 포함된 것으로 본다. 정류장별 첫차 통과시각(beginTm)은
//     같은 차량의 궤적이 아니라(마을버스 span 1,400분 실측) 검증에 못 쓴다. 검증은 OTP 경로 소요시간과 외부 길찾기 대조로 한다.
//   - ServiceStart/End: 생성 시점 기준 유효 기간. 재생성 때마다 갱신.
const (
	FallbackSpeedKmh = 18.0
	DwellSec         = 0
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
	RouteID, Name string
	NStops        int
	TravelSec     int // 구간거리÷속도 합
	Skipped       string
}

type Report struct {
	Routes       []RouteReport
	NBusRoutes   int
	NBusStops    int
	NSubwayTrips int
	NSubwayStops int
}

// Build 는 out 에 GTFS zip 을 쓴다. subway 는 nil 이면 버스만 쓴다.
func Build(out string, buses []BusRoute, subway *ktdb.Subway) (*Report, error) {
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
		stopRows = append(stopRows, s)
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
		for _, s := range subway.Stops {
			stopRows = append(stopRows, []string{s["stop_id"], s["stop_name"], s["stop_lat"], s["stop_lon"]})
		}
		tr := [][]string{}
		for _, t := range subway.Transfers {
			tr = append(tr, []string{t["from_stop_id"], t["to_stop_id"], t["transfer_type"], t["min_transfer_time"]})
		}
		w.table("transfers.txt", []string{"from_stop_id", "to_stop_id", "transfer_type", "min_transfer_time"}, tr)
		rep.NSubwayTrips = len(subway.Trips)
		rep.NSubwayStops = len(subway.Stops)
	}

	w.table("routes.txt", []string{"route_id", "agency_id", "route_short_name", "route_long_name", "route_type"}, routes)
	w.table("trips.txt", []string{"route_id", "service_id", "trip_id", "trip_headsign", "direction_id"}, trips)
	w.table("stop_times.txt", []string{"trip_id", "arrival_time", "departure_time", "stop_id", "stop_sequence"}, stopTimes)
	w.table("frequencies.txt", []string{"trip_id", "start_time", "end_time", "headway_secs", "exact_times"}, freqs)
	w.table("stops.txt", []string{"stop_id", "stop_name", "stop_lat", "stop_lon"}, stopRows)
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
// trip 은 둘이다: (1) `_T` — 첫 정류장 00:00:00 기준 상대시각 + frequencies(첫차~막차, exact_times=0).
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
	tripID := routeID + "_T"
	lastTripID := routeID + "_LAST"
	routes = append(routes, []string{routeID, BusAgencyID, r.Name, r.StartName + " ~ " + r.EndName, "3"})
	*trips = append(*trips, []string{routeID, "ALL", tripID, r.EndName, "0"})
	*trips = append(*trips, []string{routeID, "ALL", lastTripID, r.EndName, "0"})
	*freqs = append(*freqs, []string{tripID, fmtTime(first), fmtTime(last), strconv.Itoa(term * 60), "0"})

	t := 0
	for i, s := range b.Stops {
		if i > 0 {
			dist, _ := strconv.Atoi(strings.TrimSpace(s.SectDist))
			spd, _ := strconv.Atoi(strings.TrimSpace(s.SectSpd))
			kmh := float64(spd)
			if kmh <= 0 {
				kmh = FallbackSpeedKmh
			}
			t += int(float64(dist)/(kmh*1000/3600)+0.5) + DwellSec
		}
		stopID := "BS_" + strings.TrimSpace(s.StationID)
		if _, ok := stops[stopID]; !ok {
			stops[stopID] = []string{stopID, s.Name, strings.TrimSpace(s.Lat), strings.TrimSpace(s.Lon)}
		}
		seq := strconv.Itoa(i + 1)
		*stopTimes = append(*stopTimes, []string{tripID, fmtTime(t), fmtTime(t), stopID, seq})
		*stopTimes = append(*stopTimes, []string{lastTripID, fmtTime(last + t), fmtTime(last + t), stopID, seq})
	}
	rep.TravelSec = t
	return busResult{routes, rep}
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
