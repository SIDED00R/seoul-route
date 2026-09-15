package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
	"github.com/SIDED00R/seoul-route/backend/internal/realtime"
)

// 폴리라인 문자열을 횡단보도 수로 바로 매핑하는 가짜 색인.
type fakeCrossings map[string]int

func (f fakeCrossings) Count(polyline string) int { return f[polyline] }

func walkLeg(poly, start, end string, toLat, toLon float64) otp.Leg {
	return otp.Leg{Mode: "WALK", Polyline: poly, Start: start, End: end, Duration: 300, Distance: 400,
		ToLat: toLat, ToLon: toLon}
}

// 도보 leg 마다 횡단보도 수 × 대기를 더하고 여정 소요·합계에 반영한다. 마지막 leg 가 도보면 여정 End 도 늦춘다.
// 탑승 여유(3분)가 대기(2곳 76초)보다 크면 재탐색하지 않는다.
func TestCrossingWaitsAddedWithoutReplan(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:30:00+09:00",
			walkLeg("a", "2026-09-14T14:00:00+09:00", "2026-09-14T14:05:00+09:00", 37.55, 126.97),
			otp.Leg{Mode: "SUBWAY", TransitLeg: true, Start: "2026-09-14T14:08:00+09:00", End: "2026-09-14T14:25:00+09:00"},
			walkLeg("b", "2026-09-14T14:25:00+09:00", "2026-09-14T14:30:00+09:00", 37.50, 127.03))}, nil
	}}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"a": 2, "b": 1}, CrossingSec: 38}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 1 {
		t.Fatalf("err=%v its=%d", err, len(its))
	}
	it := its[0]
	if it.CrossingWait != 114 || it.Duration != 1800+114 || it.Replanned || it.End != "2026-09-14T14:30:38+09:00" {
		t.Fatalf("it=%+v", it)
	}
	if it.Legs[0].Crossings != 2 || it.Legs[0].CrossingWait != 76 || it.Legs[0].Duration != 376 ||
		it.Legs[2].Crossings != 1 || it.Legs[2].Duration != 338 || it.Legs[1].Crossings != 0 {
		t.Fatalf("legs=%+v", it.Legs)
	}
	if len(f.calls) != 3 { // 전체·지하철만·버스만. 재탐색 호출 없음
		t.Fatalf("OTP 호출 %d", len(f.calls))
	}
}

// 대기(3곳 114초)가 탑승 여유(1분)를 넘기면 도보 도착 좌표에서 늦어진 시각으로 다시 탐색해 뒤 구간을 갈아 끼운다.
func TestCrossingWaitReplansMissedBoarding(t *testing.T) {
	var replan []otp.Request
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		if r.Depart != nil && r.Origin.Lat == 37.55 { // 재탐색: 도보 끝 좌표에서 출발
			replan = append(replan, r)
			return []otp.Itinerary{itin("2026-09-14T14:06:54+09:00", "2026-09-14T14:40:00+09:00",
				otp.Leg{Mode: "SUBWAY", TransitLeg: true, Start: "2026-09-14T14:12:00+09:00", End: "2026-09-14T14:36:00+09:00"},
				walkLeg("c", "2026-09-14T14:36:00+09:00", "2026-09-14T14:40:00+09:00", 37.50, 127.03))}, nil
		}
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:30:00+09:00",
			walkLeg("a", "2026-09-14T14:00:00+09:00", "2026-09-14T14:05:00+09:00", 37.55, 126.97),
			otp.Leg{Mode: "SUBWAY", TransitLeg: true, FromStopID: "seoul:ST_A", Start: "2026-09-14T14:06:00+09:00",
				End: "2026-09-14T14:25:00+09:00"},
			walkLeg("b", "2026-09-14T14:25:00+09:00", "2026-09-14T14:30:00+09:00", 37.50, 127.03))}, nil
	}}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"a": 3, "b": 1, "c": 2}, CrossingSec: 38}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 1 {
		t.Fatalf("err=%v its=%d", err, len(its))
	}
	it := its[0]
	if !it.Replanned || len(it.Legs) != 3 || it.Legs[2].Polyline != "c" ||
		it.Legs[1].Start != "2026-09-14T14:12:00+09:00" {
		t.Fatalf("갈아 끼운 뒤 구간이 아니다: %+v", it.Legs)
	}
	// 소요 = 14:00 → 14:40 + 뒤 도보 대기 76초. 대기 합 = 앞 도보 114 + 뒤 도보 76
	if it.Duration != 2400+76 || it.CrossingWait != 190 || it.End != "2026-09-14T14:41:16+09:00" || it.Transfers != 0 {
		t.Fatalf("it=%+v", it)
	}
	if len(replan) != 1 || replan[0].Depart.Format("15:04:05") != "14:06:54" || !replan[0].Modes.TransitOnly ||
		replan[0].OriginStop != "seoul:ST_A" {
		t.Fatalf("재탐색 요청: %+v", replan)
	}
}

// 재탐색 결과가 재탐색하지 않은 다른 후보와 같은 차(노선·탑승 정류장)를 타면 열등 복제라 뺀다.
func TestCrossingReplanDropsDuplicateOfExistingTransit(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		if r.Depart != nil && r.OriginStop == "seoul:BS_1" { // 버스 정류장에서 재탐색: 역으로 되돌아가 같은 2호선
			return []otp.Itinerary{itin("2026-09-14T14:06:54+09:00", "2026-09-14T14:40:00+09:00",
				walkLeg("back", "2026-09-14T14:06:54+09:00", "2026-09-14T14:09:00+09:00", 37.55, 126.97),
				otp.Leg{Mode: "SUBWAY", Route: "2호선", FromStopID: "seoul:ST_A", TransitLeg: true,
					Start: "2026-09-14T14:12:00+09:00", End: "2026-09-14T14:40:00+09:00"})}, nil
		}
		return []otp.Itinerary{
			itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:30:00+09:00",
				otp.Leg{Mode: "SUBWAY", Route: "2호선", FromStopID: "seoul:ST_A", TransitLeg: true,
					Start: "2026-09-14T14:00:00+09:00", End: "2026-09-14T14:30:00+09:00"}),
			itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:35:00+09:00",
				walkLeg("a", "2026-09-14T14:00:00+09:00", "2026-09-14T14:05:00+09:00", 37.55, 126.97),
				otp.Leg{Mode: "BUS", Route: "472", FromStopID: "seoul:BS_1", TransitLeg: true,
					Start: "2026-09-14T14:06:00+09:00", End: "2026-09-14T14:35:00+09:00"}),
		}, nil
	}}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"a": 3}, CrossingSec: 38}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 1 || its[0].Replanned || its[0].Legs[0].Route != "2호선" {
		t.Fatalf("복제된 재탐색 결과가 남았다: err=%v %+v", err, its)
	}
}

// 재탐색이 경로를 못 찾으면 후보는 그대로(놓친 채) 남는다. 색인이 없으면 아무것도 안 한다.
func TestCrossingReplanFailureKeepsItinerary(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		if r.Depart != nil && r.Origin.Lat == 37.55 {
			return nil, otp.ErrNoRoute
		}
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:30:00+09:00",
			walkLeg("a", "2026-09-14T14:00:00+09:00", "2026-09-14T14:05:00+09:00", 37.55, 126.97),
			otp.Leg{Mode: "SUBWAY", TransitLeg: true, Start: "2026-09-14T14:06:00+09:00",
				End: "2026-09-14T14:30:00+09:00"})}, nil
	}}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"a": 3}, CrossingSec: 38}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 1 || its[0].Replanned || its[0].CrossingWait != 114 || its[0].Duration != 1914 {
		t.Fatalf("err=%v its=%+v", err, its)
	}
	p = &Planner{OTP: f}
	its, _ = p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if its[0].CrossingWait != 0 || its[0].Duration != 1800 {
		t.Fatalf("색인 없음: %+v", its[0])
	}
}

// 경유지 요청은 재탐색하지 않는다(재탐색이 남은 경유지를 버린다). 대기는 그대로 더한다.
func TestCrossingNoReplanWithVia(t *testing.T) {
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		if r.OriginStop == "seoul:ST_A" {
			t.Fatalf("경유지 요청에서 재탐색 호출: %+v", r)
		}
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:30:00+09:00",
			walkLeg("a", "2026-09-14T14:00:00+09:00", "2026-09-14T14:05:00+09:00", 37.55, 126.97),
			otp.Leg{Mode: "SUBWAY", TransitLeg: true, FromStopID: "seoul:ST_A", Start: "2026-09-14T14:06:00+09:00",
				End: "2026-09-14T14:30:00+09:00"})}, nil
	}}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"a": 3}, CrossingSec: 38}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam,
		Via: []Point{{Lat: 37.52, Lon: 126.92}}, Modes: []SegmentMode{ModeAny, ModeAny}})
	if err != nil || len(its) == 0 || its[0].Replanned || its[0].CrossingWait != 114 || its[0].Duration != 1914 {
		t.Fatalf("err=%v its=%+v", err, its)
	}
}

// 따릉이 접근(도보→자전거→도보) 뒤 탑승: 자전거 leg 대기 76초만으로는 여유(1분)를 넘기지 못하는 게 아니라, 탑승 앞 연속
// 구간의 대기 합(76+38=114)으로 판정하고 재탐색 출발 시각도 그 합만큼 늦춘다. 재탐색은 탑승 직전 leg 뒤에서 시작한다.
func TestCrossingWaitAccumulatesBeforeBoarding(t *testing.T) {
	var replan []otp.Request
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		if r.OriginStop == "seoul:BS_1" {
			replan = append(replan, r)
			return []otp.Itinerary{itin("2026-09-14T14:08:54+09:00", "2026-09-14T14:40:00+09:00",
				otp.Leg{Mode: "BUS", Route: "472", FromStopID: "seoul:BS_1", TransitLeg: true,
					Start: "2026-09-14T14:15:00+09:00", End: "2026-09-14T14:40:00+09:00"})}, nil
		}
		bike := otp.Leg{Mode: "BICYCLE", Polyline: "bike", Start: "2026-09-14T14:02:00+09:00",
			End: "2026-09-14T14:05:00+09:00", Duration: 180, ToLat: 37.55, ToLon: 126.97}
		return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:30:00+09:00",
			walkLeg("w1", "2026-09-14T14:00:00+09:00", "2026-09-14T14:02:00+09:00", 37.551, 126.971),
			bike,
			walkLeg("w2", "2026-09-14T14:05:00+09:00", "2026-09-14T14:07:00+09:00", 37.55, 126.97),
			otp.Leg{Mode: "BUS", Route: "472", FromStopID: "seoul:BS_1", TransitLeg: true,
				Start: "2026-09-14T14:08:00+09:00", End: "2026-09-14T14:30:00+09:00"})}, nil
	}}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"bike": 2, "w2": 1}, CrossingSec: 38}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 1 || !its[0].Replanned {
		t.Fatalf("누적 대기 114초 > 여유 60초인데 재탐색이 없다: err=%v its=%+v", err, its)
	}
	if len(replan) != 1 || replan[0].Depart.Format("15:04:05") != "14:08:54" || replan[0].Origin.Lat != 37.55 {
		t.Fatalf("재탐색 출발은 마지막 도보 끝(14:07)+누적 114초, 그 도착 좌표여야: %+v", replan)
	}
	if legs := its[0].Legs; len(legs) != 4 || legs[2].Polyline != "w2" || legs[3].Start != "2026-09-14T14:15:00+09:00" {
		t.Fatalf("도보→자전거→도보를 살리고 탑승만 갈아 끼워야: %+v", legs)
	}
	// 탑승 직전 도보에 횡단보도가 없어도(자전거 3곳 114초만) 누적으로 판정한다.
	replan = nil
	p = &Planner{OTP: f, Crossings: fakeCrossings{"bike": 3}, CrossingSec: 38}
	its, err = p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 1 || !its[0].Replanned || len(replan) != 1 ||
		replan[0].Depart.Format("15:04:05") != "14:08:54" {
		t.Fatalf("직전 도보 대기 0 이어도 누적 114초로 재탐색해야: err=%v replan=%+v its=%+v", err, replan, its)
	}
}

// 도착지가 역 ID 로 앵커링된 요청은 재탐색 결과에도 이탈 60초가 남는다(End·Duration). 대조군: 재탐색 없는 같은 요청.
func TestCrossingReplanKeepsStationExitSlack(t *testing.T) {
	mk := func(crossings int) *Planner {
		f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
			if r.OriginStop == "seoul:ST_A" {
				return []otp.Itinerary{itin("2026-09-14T14:06:54+09:00", "2026-09-14T14:40:00+09:00",
					otp.Leg{Mode: "SUBWAY", Route: "9호선", FromStopID: "seoul:ST_A", TransitLeg: true,
						Start: "2026-09-14T14:12:00+09:00", End: "2026-09-14T14:40:00+09:00"})}, nil
			}
			return []otp.Itinerary{itin("2026-09-14T14:00:00+09:00", "2026-09-14T14:30:00+09:00",
				walkLeg("a", "2026-09-14T14:00:00+09:00", "2026-09-14T14:05:00+09:00", 37.55, 126.97),
				otp.Leg{Mode: "SUBWAY", Route: "1호선", FromStopID: "seoul:ST_A", TransitLeg: true,
					Start: "2026-09-14T14:06:00+09:00", End: "2026-09-14T14:30:00+09:00"})}, nil
		}}
		p := &Planner{OTP: f, Crossings: fakeCrossings{"a": crossings}, CrossingSec: 38}
		p.SetStations([]otp.Station{{ID: "seoul:ST_강남", Name: "강남", Lat: gangnam.Lat, Lon: gangnam.Lon}})
		return p
	}
	dest := Point{Lat: gangnam.Lat, Lon: gangnam.Lon, Name: "강남역"}
	its, err := mk(1).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: dest})
	if err != nil || its[0].Replanned || its[0].End != "2026-09-14T14:31:00+09:00" || its[0].Duration != 1898 {
		t.Fatalf("대조군(재탐색 없음): err=%v %+v", err, its)
	}
	its, err = mk(3).Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: dest})
	if err != nil || !its[0].Replanned || its[0].End != "2026-09-14T14:41:00+09:00" || its[0].Duration != 2460 {
		t.Fatalf("재탐색 뒤 이탈 60초가 빠졌다: err=%v %+v", err, its)
	}
}

// 실시간 보정(Start·End 를 옮기고 소요를 다시 잼)이 탑승 앞 도보의 횡단보도 대기를 여정 Duration 에서 지우지 않는다.
func TestCrossingWaitSurvivesRealtime(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)
	now := time.Date(2026, 9, 15, 14, 0, 0, 0, kst)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"msgHeader":{"headerCd":"0","headerMsg":"ok"},"msgBody":{"itemList":[
		 {"stId":"1","arrmsg1":"11분후[8번째 전]","exps1":"660","arrmsg2":"출발대기","exps2":"0"}]}}`))
	}))
	defer srv.Close()
	f := &fakeOTP{answer: func(r otp.Request) ([]otp.Itinerary, error) {
		return []otp.Itinerary{itin("2026-09-15T14:00:00+09:00", "2026-09-15T14:30:00+09:00",
			walkLeg("a", "2026-09-15T14:00:00+09:00", "2026-09-15T14:05:00+09:00", 37.55, 126.97),
			otp.Leg{Mode: "BUS", Route: "402", RouteID: "seoul:B_100100063", FromStopID: "seoul:BS_1",
				TransitLeg: true, Start: "2026-09-15T14:08:00+09:00", End: "2026-09-15T14:30:00+09:00"})}, nil
	}}
	c := &realtime.Corrector{Bus: &realtime.BusClient{Key: "k", HTTP: srv.Client(), Base: srv.URL},
		Now: func() time.Time { return now }}
	p := &Planner{OTP: f, Crossings: fakeCrossings{"a": 1}, CrossingSec: 38, Now: func() time.Time { return now },
		Realtime: c}
	its, err := p.Plan(context.Background(), PlanRequest{Origin: seoulStn, Destination: gangnam})
	if err != nil || len(its) != 1 {
		t.Fatalf("err=%v its=%d", err, len(its))
	}
	it := its[0]
	if !it.Realtime || it.Replanned || it.RealtimeDelta != 180 {
		t.Fatalf("실시간 +3분 보정·재탐색 없음이 전제: %+v", it)
	}
	// 실시간 3분은 출발을 미루는 것(DepartIn)이라 소요는 시간표 30분 + 횡단보도 38초 그대로여야 한다.
	if it.CrossingWait != 38 || it.DepartIn != 180 || it.Duration != 1800+38 {
		t.Fatalf("Duration 에서 대기가 사라졌다: %+v", it)
	}
}
