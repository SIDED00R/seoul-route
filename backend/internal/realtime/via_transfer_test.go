package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

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
