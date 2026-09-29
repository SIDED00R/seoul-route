package otp

import (
	"math"
	"strings"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/crossing"
)

// joinInterlined 는 같은 열차를 내리지 않고 이어 탄 구간 b(OTP interlineWithPreviousLeg — 생성 GTFS block_id, 2호선 성수)를
// 앞 구간 a 에 붙인 한 구간을 돌려준다. 이어 탄 역(a 의 하차역, 정류장 jointID)은 중간 정차가 되고, 탑승 쪽 정보(출발역·
// 행선지·다음 정차·앞뒤 차)는 a 의 것을 쓴다.
func joinInterlined(a, b Leg, jointID string) Leg {
	out := a
	out.ToName, out.ToLat, out.ToLon, out.End = b.ToName, b.ToLat, b.ToLon, b.End
	out.Distance += b.Distance
	out.Duration += b.Duration
	joint := Stop{Name: a.ToName, Lat: a.ToLat, Lon: a.ToLon, StopID: jointID}
	shift := 0 // b 출발의 a 출발 기준 초
	if start, err := time.Parse(time.RFC3339, a.Start); err == nil {
		if e, err := time.Parse(time.RFC3339, a.End); err == nil {
			joint.OffsetSec = int(e.Sub(start).Seconds())
		}
		if s, err := time.Parse(time.RFC3339, b.Start); err == nil {
			shift = int(s.Sub(start).Seconds())
		}
		if e, err := time.Parse(time.RFC3339, b.End); err == nil {
			out.Duration = e.Sub(start).Seconds()
		}
	}
	out.Stops = append(append([]Stop(nil), a.Stops...), joint)
	for _, s := range b.Stops {
		s.OffsetSec += shift
		out.Stops = append(out.Stops, s)
	}
	out.Polyline = joinPolylines(a.Polyline, b.Polyline)
	return out
}

// joinPolylines 는 두 encoded polyline 을 이어 하나로 인코딩한다. b 의 첫 점이 a 의 끝점과 같으면 한 번만 넣는다.
func joinPolylines(a, b string) string {
	pa, pb := crossing.DecodePolyline(a), crossing.DecodePolyline(b)
	if len(pa) > 0 && len(pb) > 0 && pa[len(pa)-1] == pb[0] {
		pb = pb[1:]
	}
	return encodePolyline(append(pa, pb...))
}

// encodePolyline 은 점 목록을 Google encoded polyline(정밀도 1e-5)으로 인코딩한다. crossing.DecodePolyline 의 역이다.
func encodePolyline(pts []crossing.Point) string {
	var b strings.Builder
	put := func(v int) {
		u := v << 1
		if v < 0 {
			u = ^u
		}
		for u >= 0x20 {
			b.WriteByte(byte((0x20 | (u & 0x1f)) + 63))
			u >>= 5
		}
		b.WriteByte(byte(u + 63))
	}
	pLat, pLon := 0, 0
	for _, p := range pts {
		lat, lon := int(math.Round(p.Lat*1e5)), int(math.Round(p.Lon*1e5))
		put(lat - pLat)
		put(lon - pLon)
		pLat, pLon = lat, lon
	}
	return b.String()
}
