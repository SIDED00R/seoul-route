package route

import (
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

func at(min int) time.Time {
	return time.Date(2026, 9, 14, 14, min, 0, 0, time.FixedZone("KST", 9*3600))
}

// OTP 는 같은 노선을 출발시각만 다르게 여러 개 돌려준다. 빔 안에서 서명이 같은 후보는 하나로 묶여야
// 빔 폭이 복제로 채워지지 않는다(실측: first=5 가 전부 402번 1분 간격).
func TestPruneCollapsesSameSignature(t *testing.T) {
	bus := func(min int) partial {
		return partial{legs: []otp.Leg{{Mode: "BUS", Route: "402", FromName: "A", ToName: "B"}}, end: at(min)}
	}
	other := partial{legs: []otp.Leg{{Mode: "BUS", Route: "150", FromName: "A", ToName: "B"}}, end: at(40)}
	out := prune([]partial{bus(31), bus(30), bus(32), other})
	if len(out) != 2 {
		t.Fatalf("복제가 묶이지 않음: %d개", len(out))
	}
	if !out[0].end.Equal(at(30)) || out[0].legs[0].Route != "402" || out[1].legs[0].Route != "150" {
		t.Fatalf("가장 이른 402 + 150 이어야: %+v", out)
	}
}

// 도착시각 하나로 자르면 사라지던 "늦지만 환승이 적은" 후보가 살아남는다.
func TestPruneKeepsNonDominated(t *testing.T) {
	mk := func(route string, min, transfers int, walk float64) partial {
		return partial{legs: []otp.Leg{{Mode: "BUS", Route: route, FromName: "A", ToName: "B"}},
			end: at(min), transfers: transfers, walk: walk}
	}
	cands := []partial{
		mk("r1", 30, 2, 500), mk("r2", 31, 2, 500), mk("r3", 32, 2, 500), mk("r4", 33, 2, 500),
		mk("r5", 45, 0, 500), // 15분 늦지만 환승 0 → 비지배
	}
	out := prune(cands)
	if len(out) != BeamWidth {
		t.Fatalf("빔 폭 %d 여야: %d", BeamWidth, len(out))
	}
	routes := map[string]bool{}
	for _, c := range out {
		routes[c.legs[0].Route] = true
	}
	if !routes["r1"] || !routes["r5"] {
		t.Fatalf("r1(최단)과 r5(환승 0)가 있어야: %v", routes)
	}
}
