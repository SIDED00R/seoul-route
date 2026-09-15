// Package crossing 은 도보·따릉이 구간이 지나는 신호 횡단보도를 세고 기대 대기를 더한다.
// 입력은 otp/extract_crossings.py 가 서울 OSM 에서 뽑은 crossings.csv(신호 있는 횡단보도 노드 좌표).
package crossing

// Point 는 위경도(도).
type Point struct{ Lat, Lon float64 }

// DecodePolyline 은 Google encoded polyline(정밀도 1e-5, OTP legGeometry.points)을 점 목록으로 푼다.
// 형식이 깨지면 그때까지 푼 점만 돌려준다.
func DecodePolyline(s string) []Point {
	var out []Point
	lat, lon := 0, 0
	i := 0
	next := func() (int, bool) {
		result, shift := 0, 0
		for {
			if i >= len(s) {
				return 0, false
			}
			b := int(s[i]) - 63
			i++
			result |= (b & 0x1f) << shift
			shift += 5
			if b < 0x20 {
				break
			}
		}
		if result&1 != 0 {
			return ^(result >> 1), true
		}
		return result >> 1, true
	}
	for i < len(s) {
		dLat, ok := next()
		if !ok {
			break
		}
		dLon, ok := next()
		if !ok {
			break
		}
		lat += dLat
		lon += dLon
		out = append(out, Point{Lat: float64(lat) / 1e5, Lon: float64(lon) / 1e5})
	}
	return out
}
