// Package headway 는 생성 GTFS zip 의 frequencies.txt 에서 노선별 배차간격을 읽는다.
// 버스는 배차간격 기반 시간표라 OTP 의 previousLegs/nextLegs 가 쓸 값을 주지 않는다(실측: 막차 trip 만 반환).
// 그래서 앱의 "배차 약 N분" 표시는 이 표에서 나온다. 지하철은 OTP 의 앞뒤 열차 시각을 쓴다.
package headway

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/SIDED00R/seoul-route/backend/internal/gtfszip"
)

// Load 는 route_id → 배차간격(초)을 돌려준다. 한 노선에 여러 행이면 가장 짧은 값(가장 잦은 배차)을 쓴다.
func Load(zipPath string) (map[string]int, error) {
	zr, err := gtfszip.Open(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	trips, found, err := zr.Rows("trips.txt")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("trips.txt 없음")
	}
	tripRoute := map[string]string{}
	for _, r := range trips {
		tripRoute[r["trip_id"]] = r["route_id"]
	}
	freqs, found, err := zr.Rows("frequencies.txt")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("frequencies.txt 없음")
	}
	out := map[string]int{}
	for _, r := range freqs {
		route, ok := tripRoute[r["trip_id"]]
		if !ok {
			continue
		}
		h, err := strconv.Atoi(strings.TrimSpace(r["headway_secs"]))
		if err != nil || h <= 0 {
			continue
		}
		if cur, ok := out[route]; !ok || h < cur {
			out[route] = h
		}
	}
	return out, nil
}
