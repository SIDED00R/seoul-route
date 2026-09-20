package route

import (
	"strings"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// annotateTransitMetadata 는 대중교통 구간에 GTFS 배차와 노선 색을 붙인다.
func (p *Planner) annotateTransitMetadata(its []otp.Itinerary) {
	if len(p.Headways) == 0 && len(p.RouteStyles) == 0 {
		return
	}
	for i := range its {
		for j := range its[i].Legs {
			leg := &its[i].Legs[j]
			if !leg.TransitLeg {
				continue
			}
			id := localRouteID(leg.RouteID)
			if headway, ok := p.Headways[id]; ok {
				leg.HeadwaySec = headway
			}
			if style, ok := p.RouteStyles[id]; ok {
				leg.Color = style.Color
				leg.TextColor = style.TextColor
			}
		}
	}
}

func localRouteID(id string) string {
	if _, local, ok := strings.Cut(id, ":"); ok {
		return local
	}
	return id
}
