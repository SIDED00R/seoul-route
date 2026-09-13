package build

import (
	"sort"
	"strconv"

	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
)

// 역 구내 통로(pathways.txt) 상수.
//   - 부모역 안 자식 stop(노선별 승강장) 사이를 OTP 가 지상 도로망으로 걷는다(승강장 좌표가 도로에서 멀어 강남
//     2호선↔신분당선 182m 가 도보 7.2분, 2026-09-13 실측). transfers.txt 의 min_transfer_time 은 하한일 뿐이라
//     도로 경로를 줄이지 못한다. 승강장끼리 walkway pathway 를 두면 그 통로가 도로보다 짧아 환승에 쓰인다.
//   - 환승 통과시간은 transfers.txt(파일럿, 역별 실측값)가 있으면 그 값, 없으면 직선거리÷PathwayWalkMps + PathwayStairSec.
//     1.0 m/s·60초는 계단·개찰 포함 역 구내 보행 placeholder(2026-09-13). 서울교통공사 환승 소요 자료로 재보정한다.
//     거리 폴백은 PathwayFallbackMaxM 까지만: 기준명+800m 로 묶인 부모역이 실제 환승역임을 보장하지 않아, 별개 역사인
//     신촌 2호선↔경의중앙선(684m)에도 통로가 생겼다(2026-09-13 실측). transfers 기반 151쌍 최대 390m, 폴백 중 실재하는
//     서울역 5쌍 최대 434m, 신촌 684m 라 그 사이 500m 를 상한으로 둔다. 상한에 걸린 쌍 수는 보고서에 남긴다.
//   - 통로가 있는 역의 승강장은 OTP 가 도로에 직접 잇지 않고 출입구(location_type 2)만 잇는다(실측: 출입구 없이
//     승강장 통로만 넣자 IsolatedStop 273→480). GTFS 검증기도 통로가 있는 역은 모든 위치가 출입구에서 닿아야 한다고
//     본다(pathway_unreachable_location). 그래서 승강장마다 같은 좌표에 출입구를 두고 진입 EntrySec·이탈 ExitSec
//     통로로 잇는다. 좌표가 같으니 도로 연결 지점은 전과 같고, 좌표 출발·도착 여정에 진입·이탈 시간이 더해진다.
//     값은 backend/internal/route/station_slack.go 의 StationEntrySec/StationExitSec(역 ID 앵커링 요청에 백엔드가
//     더하는 값)와 같아야 두 요청 방식의 소요가 일치한다 — 단 출입구가 생기는 자식 2개 이상 역(104역)에서만이고,
//     단일 승강장 역(533역)은 좌표 요청에 진입·이탈이 붙지 않는다(전 역 출입구는 #27 실제 출입구 좌표 뒤에 재검토).
//   - transfers.txt 쌍인데 부모역이 갈린 경우(도봉산 1↔7호선, 파일럿 1호선 좌표가 1.1km 남쪽)는 통로를 만들지 않고
//     개수만 보고한다(unpairedTransfers).
const (
	PathwayWalkMps      = 1.0
	PathwayStairSec     = 60
	PathwayFallbackMaxM = 500.0
	EntrySec            = 120
	ExitSec             = 60
)

// stationPathways 는 자식 stop 이 2개 이상인 부모역마다 (1) 승강장별 출입구 stop 행(stop_id, stop_name, lat, lon,
// location_type 2, parent_station) (2) 출입구→승강장 EntrySec·승강장→출입구 ExitSec 단방향 통로 (3) 승강장 쌍마다
// 양방향 walkway 통로를 만든다. 통로 행: pathway_id, from_stop_id, to_stop_id, pathway_mode(1), is_bidirectional,
// traversal_time(초). stop_id 정렬로 출력이 결정적이다. farPairs 는 transfers 값이 없고 PathwayFallbackMaxM 을 넘어
// 통로를 만들지 않은 쌍 수.
func stationPathways(stops []ktdb.Row, parentOf map[string]string, transfers []ktdb.Row) (
	entrances, pathways [][]string, farPairs int) {
	minTime := map[[2]string]int{}
	for _, t := range transfers {
		if n, err := strconv.Atoi(t["min_transfer_time"]); err == nil && n > 0 {
			minTime[[2]string{t["from_stop_id"], t["to_stop_id"]}] = n
		}
	}
	byParent := map[string][]ktdb.Row{}
	for _, s := range stops {
		if p := parentOf[s["stop_id"]]; p != "" {
			byParent[p] = append(byParent[p], s)
		}
	}
	parents := make([]string, 0, len(byParent))
	for p := range byParent {
		parents = append(parents, p)
	}
	sort.Strings(parents)
	for _, p := range parents {
		kids := byParent[p]
		if len(kids) < 2 {
			continue
		}
		sort.Slice(kids, func(i, j int) bool { return kids[i]["stop_id"] < kids[j]["stop_id"] })
		for _, k := range kids {
			id, en := k["stop_id"], "EN_"+k["stop_id"]
			entrances = append(entrances, []string{en, k["stop_name"], k["stop_lat"], k["stop_lon"], "2", p})
			pathways = append(pathways,
				[]string{"PWI_" + id, en, id, "1", "0", strconv.Itoa(EntrySec)},
				[]string{"PWO_" + id, id, en, "1", "0", strconv.Itoa(ExitSec)})
		}
		for i := 0; i < len(kids); i++ {
			for j := i + 1; j < len(kids); j++ {
				a, b := kids[i], kids[j]
				sec, ok := minTime[[2]string{a["stop_id"], b["stop_id"]}]
				if !ok {
					sec, ok = minTime[[2]string{b["stop_id"], a["stop_id"]}]
				}
				if !ok {
					d := haversineM(parseF(a["stop_lat"]), parseF(a["stop_lon"]), parseF(b["stop_lat"]), parseF(b["stop_lon"]))
					if d > PathwayFallbackMaxM {
						farPairs++
						continue
					}
					sec = int(d/PathwayWalkMps) + PathwayStairSec
				}
				pathways = append(pathways, []string{"PW_" + a["stop_id"] + "_" + b["stop_id"], a["stop_id"], b["stop_id"],
					"1", "1", strconv.Itoa(sec)})
			}
		}
	}
	return entrances, pathways, farPairs
}

// unpairedTransfers 는 transfers.txt 행 중 두 stop 의 부모역이 달라 통로가 생기지 않는 행의 수.
func unpairedTransfers(transfers []ktdb.Row, parentOf map[string]string) int {
	n := 0
	for _, t := range transfers {
		if parentOf[t["from_stop_id"]] != parentOf[t["to_stop_id"]] {
			n++
		}
	}
	return n
}
