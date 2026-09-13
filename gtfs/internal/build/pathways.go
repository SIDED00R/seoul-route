package build

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
	"github.com/SIDED00R/seoul-route/gtfs/internal/osm"
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
//     본다(pathway_unreachable_location).
//   - 출입구는 OSM railway=subway_entrance(otp/data/subway-entrances.csv, 2026-09-13 2,457개, 공사 중 3개는 추출에서
//     제외)를 가장 가까운 승강장이
//     EntranceMatchM 안에 있는 역에 붙인다. 부모역 평균 좌표 기준으로 하면 승강장이 684m 떨어진 신촌은 부모에서 출입구까지
//     302m 라 실제 출구 10개가 전부 탈락했다(2026-09-13 실측, 승강장 기준 350m 로 454역·출입구 2,449개 매칭).
//     출입구↔승강장 통로는 EntranceEntrySec/EntranceExitSec + 직선거리÷PathwayWalkMps, 승강장이 출입구에서
//     PathwayFallbackMaxM 안일 때만(신촌 2호선 출구가 684m 떨어진 경의중앙선 승강장과 이어지지 않게. 현 데이터 최대 497m).
//     출입구가 도로 위 실제 위치라 좌표 출발 여정의 접근 도보가 실제 출구까지로 잡힌다(이슈 #27). OSM 출입구가 없는
//     역(bbox 밖 등)은 자식 2개 이상일 때만 승강장 좌표에 출입구를 두고 진입 EntrySec·이탈 ExitSec 통로로 잇는다
//     (이전 방식, 환승 통로 유지용). 통로가 있는 역에서 출입구 통로를 하나도 못 받은 승강장은 OTP 가 도로에 잇지 않아
//     고립되므로 그 수를 보고한다(NoEntrancePlatforms, 현 데이터 0).
//     EntrySec/ExitSec 는 backend/internal/route/station_slack.go 의 StationEntrySec/StationExitSec(역 ID 앵커링
//     요청에 백엔드가 더하는 값)와 같은 값이다.
//   - transfers.txt 쌍인데 부모역이 갈린 경우(도봉산 1↔7호선, 파일럿 1호선 좌표가 1.1km 남쪽)는 통로를 만들지 않고
//     개수만 보고한다(unpairedTransfers).
const (
	PathwayWalkMps      = 1.0
	PathwayStairSec     = 60
	PathwayFallbackMaxM = 500.0
	EntrySec            = 120
	ExitSec             = 60
	// 350m: 청담역 출구 8개가 KTDB 승강장 좌표에서 264~343m(OSM stop_area 관계로 청담역 출구 확인), 동작 9번 255m.
	// 250m 면 250~350m 대 16개(청담 8·잠실 2·마곡 2·예술회관 1·동작 1·왕십리 1·의정부 1)가 탈락한다(2026-09-13 실측).
	// 350m 에서 오부착 1건(상왕십리 4번 출구가 상왕십리 승강장 862m·왕십리 326m 라 왕십리에 붙음)은 감수.
	// OSM stop_area 관계 매칭은 관계명 표기가 제각각이라 별도 과제.
	EntranceMatchM   = 350.0
	EntranceEntrySec = 60 // 실제 출입구→승강장: 계단·개찰 상수 + 거리÷PathwayWalkMps. placeholder(2026-09-13), Phase 3 궤적으로 재보정
	EntranceExitSec  = 30 // 승강장→실제 출입구
)

// pathwayStats 는 stationPathways 가 보고서용으로 센 수.
type pathwayStats struct {
	FarPairs         int // transfers 값 없이 PathwayFallbackMaxM 을 넘어 통로를 안 만든 승강장 쌍
	RealEntranceStns int // OSM 출입구가 붙은 부모역
	FallbackStns     int // OSM 출입구가 없어 승강장 좌표 출입구로 대신한 부모역(자식 2개 이상)
	// OSM 출입구가 붙은 역인데 PathwayFallbackMaxM 안에 출입구가 없어 통로를 못 받은 승강장. 0 이 아니면 그 승강장은 고립된다
	NoEntrancePlatforms int
}

// stationPathways 는 부모역마다 출입구 stop 행(stop_id, stop_name, lat, lon, location_type 2, parent_station)과
// 통로 행(pathway_id, from_stop_id, to_stop_id, pathway_mode 1, is_bidirectional, traversal_time 초)을 만든다.
// OSM 출입구가 붙은 역은 출입구마다 PathwayFallbackMaxM 안 승강장과 진입·이탈 단방향 통로, 없으면 자식 2개 이상일 때
// 승강장 좌표 출입구.
// 자식 2개 이상이면 승강장 쌍마다 양방향 walkway. parents 는 stationGroups 의 부모 행. stop_id·출구 번호 정렬로
// 출력이 결정적이다.
func stationPathways(stops []ktdb.Row, parents [][]string, parentOf map[string]string, transfers []ktdb.Row,
	osmEntrances []osm.Entrance) (entrances, pathways [][]string, st pathwayStats) {
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
	baseOf := map[string]string{}
	for _, p := range parents {
		baseOf[p[0]] = p[1]
	}
	matched := matchEntrances(stops, parentOf, osmEntrances)
	ids := make([]string, 0, len(byParent))
	for p := range byParent {
		ids = append(ids, p)
	}
	sort.Strings(ids)
	for _, p := range ids {
		kids := byParent[p]
		sort.Slice(kids, func(i, j int) bool { return kids[i]["stop_id"] < kids[j]["stop_id"] })
		switch {
		case len(matched[p]) > 0:
			st.RealEntranceStns++
			linked := map[string]bool{}
			for k, e := range matched[p] {
				en := fmt.Sprintf("EN_%s_%d", p, k+1)
				name := entranceName(baseOf[p], e)
				entrances = append(entrances, []string{en, name, fmtCoord(e.Lat), fmtCoord(e.Lon), "2", p})
				for _, kid := range kids {
					id := kid["stop_id"]
					d := haversineM(e.Lat, e.Lon, parseF(kid["stop_lat"]), parseF(kid["stop_lon"]))
					if d > PathwayFallbackMaxM {
						continue
					}
					linked[id] = true
					walk := int(d / PathwayWalkMps)
					pathways = append(pathways,
						[]string{"PWI_" + en + "_" + id, en, id, "1", "0", strconv.Itoa(EntranceEntrySec + walk)},
						[]string{"PWO_" + en + "_" + id, id, en, "1", "0", strconv.Itoa(EntranceExitSec + walk)})
				}
			}
			for _, kid := range kids {
				if !linked[kid["stop_id"]] {
					st.NoEntrancePlatforms++
				}
			}
		case len(kids) >= 2:
			st.FallbackStns++
			for _, k := range kids {
				id, en := k["stop_id"], "EN_"+k["stop_id"]
				entrances = append(entrances, []string{en, k["stop_name"], k["stop_lat"], k["stop_lon"], "2", p})
				pathways = append(pathways,
					[]string{"PWI_" + id, en, id, "1", "0", strconv.Itoa(EntrySec)},
					[]string{"PWO_" + id, id, en, "1", "0", strconv.Itoa(ExitSec)})
			}
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
						st.FarPairs++
						continue
					}
					sec = int(d/PathwayWalkMps) + PathwayStairSec
				}
				pathways = append(pathways, []string{"PW_" + a["stop_id"] + "_" + b["stop_id"], a["stop_id"], b["stop_id"],
					"1", "1", strconv.Itoa(sec)})
			}
		}
	}
	return entrances, pathways, st
}

var exitRefRe = regexp.MustCompile(`^[0-9]+(-[0-9]+)?$`)

// entranceName: 출구 번호 형식("3", "5-1")이면 "<역명> N번 출구". 그 밖의 ref("엘리베이터", "한강진역 2번출구")는
// OSM name 이 있으면 그대로, 없으면 "<역명> 출입구". ref 가 비면 name 을 보지 않고 "<역명> 출입구"(name 이 "3" 뿐인 노드 실재).
func entranceName(base string, e osm.Entrance) string {
	switch {
	case exitRefRe.MatchString(e.Ref):
		return base + " " + e.Ref + "번 출구"
	case e.Ref != "" && e.Name != "":
		return e.Name
	default:
		return base + " 출입구"
	}
}

// matchEntrances 는 OSM 출입구를 EntranceMatchM 안에서 가장 가까운 승강장의 부모역에 붙인다. 승강장은 stop_id 순으로
// 보아 동률에서도 결정적이다. 역마다 출구 번호(숫자 우선)·좌표순.
func matchEntrances(stops []ktdb.Row, parentOf map[string]string, ents []osm.Entrance) map[string][]osm.Entrance {
	sorted := append([]ktdb.Row(nil), stops...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i]["stop_id"] < sorted[j]["stop_id"] })
	out := map[string][]osm.Entrance{}
	for _, e := range ents {
		best, bestD := "", EntranceMatchM
		for _, s := range sorted {
			p := parentOf[s["stop_id"]]
			if p == "" {
				continue
			}
			if d := haversineM(e.Lat, e.Lon, parseF(s["stop_lat"]), parseF(s["stop_lon"])); d < bestD {
				best, bestD = p, d
			}
		}
		if best != "" {
			out[best] = append(out[best], e)
		}
	}
	for p := range out {
		es := out[p]
		sort.Slice(es, func(i, j int) bool {
			ni, ei := strconv.Atoi(es[i].Ref)
			nj, ej := strconv.Atoi(es[j].Ref)
			switch {
			case ei == nil && ej == nil && ni != nj:
				return ni < nj
			case (ei == nil) != (ej == nil):
				return ei == nil
			case es[i].Ref != es[j].Ref:
				return es[i].Ref < es[j].Ref
			case es[i].Lat != es[j].Lat:
				return es[i].Lat < es[j].Lat
			default:
				return es[i].Lon < es[j].Lon
			}
		})
	}
	return out
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
