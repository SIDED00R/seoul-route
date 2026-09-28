package route

import "time"

// 경유지 체류(Point.StayMin) 규칙.
//   - 체류가 하나라도 있으면 OTP 의 via 탐색을 쓰지 않고 구간별 탐색(segmented)만 한다. OTP via 의 minimumWaitTime 은
//     기다림을 경유지가 아니라 다음 탑승 정류장에 둔다. 구간별 탐색은 구간마다 OTP 를 따로 불러 따릉이를 경유지 전에
//     반납한다.
//   - 체류한 경유지 다음 구간은 앞 구간 도착 + 체류 시각에 출발한다.
//   - 체류한 경유지에서 이어 붙이는 것은 환승으로 세지 않는다.
//   - 경유지에 닿는 leg 에 경유지 번호(1부터)와 체류 초를 붙인다(Leg.StayVia·StaySec). 앱 안내가 체류 상태에 쓴다.

// MaxStayMin 은 경유지 하나의 체류 입력 상한(분). 앱은 안내 시작 후 6시간 안의 진행 중 안내만 되살린다
// (app GuideStore.maxAge).
const MaxStayMin = 180

func hasStay(via []Point) bool {
	for _, v := range via {
		if v.StayMin > 0 {
			return true
		}
	}
	return false
}

func stayOf(v Point) time.Duration { return time.Duration(v.StayMin) * time.Minute }
