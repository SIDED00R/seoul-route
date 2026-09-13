package route

import (
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
)

// 역 진입·이탈 시간(초). 역 앵커링(anchor.go)은 출발·도착지를 역 ID 로 넘겨 OTP 가 승강장에서 바로 타고 내리게 하므로
// 출입구↔승강장 이동이 여정에서 빠진다. ODsay 대조(docs/eval/2026-09-13-0541.md, 2026-09-13)에서 지하철 직행 8구간의
// 우리 소요가 중앙값 2분 짧았던 만큼을 진입 2분 + 이탈 1분으로 나눠 넣은 초기값이다. Phase 3 궤적(출입구→첫 탑승)으로
// 재보정한다. OTP 의 boardSlack/alightSlack 은 역 ID 출발·도착에서는 소요시간에 드러나지 않고 환승에만 붙어(같은 문서 실측)
// 쓰지 않는다. 이 값을 바꾸면 gtfs/internal/build/pathways.go 의 EntrySec/ExitSec(좌표 요청에 OTP 통로가 더하는 값,
// 자식 2개 이상 역의 출입구 통로)도 같이 바꾸고 GTFS·그래프를 재생성한다.
const (
	StationEntrySec = 120.0
	StationExitSec  = 60.0
)

// applyStationSlack 은 앵커링된 출발지의 여정 출발(Start)을 진입시간만큼 앞당기고(출입구에서 출발),
// 앵커링된 도착지의 도착(End)을 이탈시간만큼 늦춘다(출입구 도착). Duration 은 그만큼 늘어난다.
// leg 의 시각은 그대로라 첫 leg 앞·끝 leg 뒤에 그만큼 틈이 생긴다(실시간 보정은 그 틈을 접근시간으로 센다).
func applyStationSlack(its []otp.Itinerary, originAnchored, destAnchored bool) {
	for i := range its {
		it := &its[i]
		if originAnchored {
			if s, err := time.Parse(time.RFC3339, it.Start); err == nil {
				it.Start = s.Add(-time.Duration(StationEntrySec) * time.Second).Format(time.RFC3339)
				it.Duration += StationEntrySec
			}
		}
		if destAnchored {
			if e, err := time.Parse(time.RFC3339, it.End); err == nil {
				it.End = e.Add(time.Duration(StationExitSec) * time.Second).Format(time.RFC3339)
				it.Duration += StationExitSec
			}
		}
	}
}

// entryDepart 는 앵커링된 출발지의 OTP 요청 시각. 출입구에서 승강장까지 가는 동안 떠나는 차는 못 타므로
// 요청 시각(nil 이면 now)을 진입시간만큼 뒤로 민다. OTP 는 이 정도 차이는 "지금 출발" 로 보고 따릉이 잔여대수를 계속 반영한다.
func entryDepart(depart *time.Time, now time.Time) *time.Time {
	t := now
	if depart != nil {
		t = *depart
	}
	t = t.Add(time.Duration(StationEntrySec) * time.Second)
	return &t
}
