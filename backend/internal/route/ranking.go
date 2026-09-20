package route

import (
	"sort"
	"strings"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 순위 페널티는 초기 제품값이며 실제 경로 선택 데이터가 쌓이면 재보정한다.
const (
	TransferPenaltySec = 240.0
	RentalPenaltySec   = 300.0
	MaxSlowerSec       = 1800.0
)

func sortByScore(its []otp.Itinerary) {
	sort.SliceStable(its, func(i, j int) bool { return score(its[i]) < score(its[j]) })
}

func setDepartIn(its []otp.Itinerary, depart *time.Time, now time.Time) {
	for i := range its {
		its[i].DepartIn = 0
		if depart != nil {
			continue
		}
		if start, err := time.Parse(time.RFC3339, its[i].Start); err == nil && start.After(now) {
			its[i].DepartIn = start.Sub(now).Seconds()
		}
	}
}

func rank(its []otp.Itinerary) []otp.Itinerary {
	if len(its) == 0 {
		return its
	}
	best := total(its[0])
	for _, it := range its[1:] {
		if total(it) < best {
			best = total(it)
		}
	}
	kept := its[:0:0]
	for _, it := range its {
		if total(it) <= best+MaxSlowerSec {
			kept = append(kept, it)
		}
	}
	sortByScore(kept)
	return kept
}

func total(it otp.Itinerary) float64 { return it.DepartIn + it.Duration }

func score(it otp.Itinerary) float64 {
	score := total(it) + TransferPenaltySec*float64(it.Transfers)
	for _, leg := range it.Legs {
		if leg.RentedBike {
			return score + RentalPenaltySec
		}
	}
	return score
}

func legSig(legs []otp.Leg) string {
	var signature strings.Builder
	for _, leg := range legs {
		signature.WriteString(leg.Mode + "|" + leg.Route + "|" + leg.FromName + "|" + leg.ToName + ";")
	}
	return signature.String()
}

func dedupe(its []otp.Itinerary) []otp.Itinerary {
	seen := map[string]int{}
	var out []otp.Itinerary
	for _, it := range its {
		sig := legSig(it.Legs)
		if index, ok := seen[sig]; ok {
			if it.Start < out[index].Start {
				out[index] = it
			}
			continue
		}
		seen[sig] = len(out)
		out = append(out, it)
	}
	return out
}
