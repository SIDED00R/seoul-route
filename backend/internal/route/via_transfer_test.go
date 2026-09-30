package route

import (
	"context"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 역 ID 로 앵커링된 경유지(체류 0)에서 구간별 탐색의 다음 구간은 앞 구간 도착 + ViaTransferSec 에 출발한다.
func TestSegmentedAnchoredViaDepartsAfterTransferSlack(t *testing.T) {
	viaCalls := 0
	f := busEachSegment(&viaCalls)
	p := &Planner{OTP: f}
	p.SetStations([]otp.Station{seoulStation})
	via := Point{Lat: 37.55407, Lon: 126.97070, Name: "서울역"}
	if _, err := p.Plan(context.Background(), PlanRequest{Origin: yeouido, Destination: gangnam,
		Via: []Point{via}}); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 9, 14, 14, 22, 0, 0, time.FixedZone("KST", 9*3600))
	found := false
	for _, c := range f.calls {
		if len(c.Via) == 0 && c.OriginStop == "seoul:ST_서울" {
			found = true
			if c.Depart == nil || !c.Depart.Equal(want) {
				t.Fatalf("둘째 구간은 14:20 도착 + 120초 = 14:22 출발: %v", c.Depart)
			}
		}
	}
	if !found {
		t.Fatalf("앵커링된 경유지에서 출발하는 구간 요청이 없다: %+v", f.calls)
	}
}

// 역 ID 로 앵커링된 경유지(체류 0)에 닿는 leg 에 환승 여유(ViaTransferSec)가 붙고, 체류가 있으면 붙지 않는다.
func TestSegmentedAnchoredViaMarksTransferSlack(t *testing.T) {
	for _, stay := range []int{0, 30} {
		viaCalls := 0
		f := busEachSegment(&viaCalls)
		p := &Planner{OTP: f}
		p.SetStations([]otp.Station{seoulStation})
		via := Point{Lat: 37.55407, Lon: 126.97070, Name: "서울역", StayMin: stay}
		its, err := p.Plan(context.Background(), PlanRequest{Origin: yeouido, Destination: gangnam, Via: []Point{via}})
		if err != nil {
			t.Fatal(err)
		}
		want := 0.0
		if stay == 0 {
			want = ViaTransferSec
		}
		found := false
		for _, it := range its {
			if len(it.Legs) == 2 && it.Legs[0].ViaTransferSec == want && it.Legs[1].ViaTransferSec == 0 {
				found = true
			}
		}
		if !found {
			t.Fatalf("체류 %d: 경유지에 닿는 leg 의 ViaTransferSec 가 %v 여야 한다: %+v", stay, want, its)
		}
	}
}
