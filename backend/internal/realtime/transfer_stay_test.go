package realtime

import (
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
