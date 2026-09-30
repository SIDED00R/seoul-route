package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 4호선(600~1200, 경유지 도착·체류 1800초) 바로 뒤에 3호선(3060 출발)이 붙은 여정.
func stayThenSubway() []otp.Leg {
	return []otp.Leg{
		{Mode: "SUBWAY", TransitLeg: true, Start: at(600), End: at(1200), StayVia: 1, StaySec: 1800},
		{Mode: "SUBWAY", TransitLeg: true, Start: at(3060), End: at(3600),
			NextDepartures: []string{at(3420), at(3780)}},
	}
}

// 체류 경유지에 닿는 지하철이 190초 늦으면 1200 + 190 + 1800 = 3190 > 3060 이라 다음 열차(3420)를 탄다.
func TestStayViaDelayMissesPlannedTrain(t *testing.T) {
	shifts, tail := laterShifts(stayThenSubway(), 0, 190*time.Second)
	if tail != 360*time.Second || shifts[1] != 360*time.Second {
		t.Fatalf("다음 열차까지 360초 밀려야 한다: shifts=%v tail=%v", shifts, tail)
	}
}

// 체류가 끝난 뒤에도 원래 열차를 탈 수 있으면 0 이다(1200 + 50 + 1800 = 3050 ≤ 3060).
func TestStayViaDelayWithinGapKeepsTrain(t *testing.T) {
	_, tail := laterShifts(stayThenSubway(), 0, 50*time.Second)
	if tail != 0 {
		t.Fatalf("tail=%v", tail)
	}
}

// 체류 경유지 뒤에 도보가 끼면 도보 끝 시각에 체류가 이미 들어 있어 체류를 다시 더하지 않는다.
func TestStayViaFollowedByWalkCountsStayOnce(t *testing.T) {
	legs := []otp.Leg{
		{Mode: "SUBWAY", TransitLeg: true, Start: at(600), End: at(1200), StayVia: 1, StaySec: 1800},
		{Mode: "WALK", Start: at(3000), End: at(3050)},
		{Mode: "SUBWAY", TransitLeg: true, Start: at(3060), End: at(3600),
			NextDepartures: []string{at(3420), at(3780)}},
	}
	_, tail := laterShifts(legs, 0, 190*time.Second)
	if tail != 360*time.Second {
		t.Fatalf("3050 + 190 = 3240 → 3420 열차(360초): tail=%v", tail)
	}
}

// 도보 leg 가 앵커링된 경유지(체류 0)에 닿으면 환승 여유 120초가 지나야 다음 열차를 탈 수 있다.
func TestViaTransferSlackAfterWalkInConnect(t *testing.T) {
	legs := []otp.Leg{
		{Mode: "SUBWAY", TransitLeg: true, Start: at(0), End: at(600)},
		{Mode: "WALK", Start: at(600), End: at(700), ViaTransferSec: 120},
		{Mode: "SUBWAY", TransitLeg: true, Start: at(830), End: at(1400),
			NextDepartures: []string{at(1130), at(1430)}},
	}
	_, tail := laterShifts(legs, 0, 100*time.Second)
	if tail != 300*time.Second {
		t.Fatalf("700 + 100 + 120 = 920 > 830 → 1130 열차(300초): tail=%v", tail)
	}
}

// 첫 탑승 앞 도보가 앵커링된 경유지에 닿으면 접근시간에 환승 여유 120초를 더해, 그 안에 오는 차는 못 탄다.
func TestViaTransferSlackInAccessTime(t *testing.T) {
	in := otp.Itinerary{Start: at(0), End: at(1300), Duration: 1300, Legs: []otp.Leg{
		{Mode: "WALK", Duration: 600, Start: at(0), End: at(600), ViaTransferSec: 120},
		{Mode: "SUBWAY", Route: "서울4호선", RouteID: "seoul:RR_4", FromName: "서울(4호선)", NextStop: "회현(4호선)",
			TransitLeg: true, Start: at(720), End: at(1300)},
	}}
	got := line4Arrival(t, 660).Adjust(context.Background(), []otp.Itinerary{in})[0]
	if got.Legs[1].Start != at(720) || got.RealtimeDelta < 0 {
		t.Fatalf("660초 열차는 접근 720초 안이라 못 탄다: start=%s delta=%v realtime=%v", got.Legs[1].Start,
			got.RealtimeDelta, got.Realtime)
	}
	in.Legs[0].ViaTransferSec = 0
	got = line4Arrival(t, 660).Adjust(context.Background(), []otp.Itinerary{in})[0]
	if got.Legs[1].Start != at(660) {
		t.Fatalf("여유가 없으면 660초 열차를 탄다: start=%s", got.Legs[1].Start)
	}
}
