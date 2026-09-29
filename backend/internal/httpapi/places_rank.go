package httpapi

import "strings"

// placesMax: /places/search 가 돌려주는 결과 수 상한.
const placesMax = 15

// kakaoPlaceDoc 은 카카오 키워드 검색 결과 한 건 중 쓰는 필드다.
type kakaoPlaceDoc struct {
	ID           string `json:"id"`
	PlaceName    string `json:"place_name"`
	RoadAddress  string `json:"road_address_name"`
	Address      string `json:"address_name"`
	Category     string `json:"category_group_name"`
	CategoryCode string `json:"category_group_code"`
	Distance     string `json:"distance"` // 요청에 x·y 가 있을 때만(m)
	X            string `json:"x"`
	Y            string `json:"y"`
}

// kakaoKeywordResult 는 카카오 키워드 검색 응답이다. same_name 은 카카오가 검색어에서 읽어 낸 지역이다
// ("강남" → keyword "", selected_region "서울 강남구").
type kakaoKeywordResult struct {
	Meta struct {
		SameName struct {
			Keyword        string `json:"keyword"`
			SelectedRegion string `json:"selected_region"`
		} `json:"same_name"`
	} `json:"meta"`
	Documents []kakaoPlaceDoc `json:"documents"`
}

// isRegionQuery 는 카카오가 검색어 전체를 지역 이름으로 읽었는지다. 그때 결과는 그 지역의 명소다
// ("강남" → 서울선릉과정릉·압구정로데오거리 …, 강남역은 15위 안에 없음).
func isRegionQuery(r kakaoKeywordResult) bool {
	return r.Meta.SameName.Keyword == "" && r.Meta.SameName.SelectedRegion != ""
}

// needsStationBoost 는 이름이 "<q>역" 인 역을 맨 위로 올릴지다: 지역 이름 검색이거나, 1위 이름이 q 로 시작하지 않을 때
// ("선릉" → 1위 서울선릉과정릉). 1위가 q 로 시작하면 카카오 순서를 둔다("강남구청" → 강남구청).
// q 가 이미 "역" 으로 끝나면 카카오가 역을 찾으므로 올리지 않는다.
func needsStationBoost(q string, main kakaoKeywordResult) bool {
	if strings.HasSuffix(q, "역") {
		return false
	}
	return isRegionQuery(main) || len(main.Documents) == 0 || !strings.HasPrefix(main.Documents[0].PlaceName, q)
}

// stationBase 는 카카오 지하철역 이름에서 노선을 뗀 역 이름이다("강남역 2호선" → "강남역").
func stationBase(name string) string {
	if i := strings.IndexByte(name, ' '); i >= 0 {
		return name[:i]
	}
	return name
}

// rankPlaces 는 카카오 결과를 길찾기 용도 순서로 합친다. 순서: 이름이 "<q>역" 인 역(needsStationBoost 일 때)
// → 지역 이름 검색이면 "<q>" 로 시작하는 역 → 카카오 정확도순.
// 같은 장소(카카오 id)는 한 번만, 지하철역(SW8)은 역 이름마다 처음 것 하나만 둔다("강남역 2호선"·"강남역 신분당선" → 앞의 것).
// 경로 탐색은 "…역" 이름을 같은 부모역으로 앵커링하므로 노선별 항목은 같은 출발·도착이 된다.
func rankPlaces(q string, main kakaoKeywordResult, stations []kakaoPlaceDoc) []kakaoPlaceDoc {
	out := make([]kakaoPlaceDoc, 0, placesMax)
	seenID, seenStation := map[string]bool{}, map[string]bool{}
	add := func(d kakaoPlaceDoc) {
		if (d.ID != "" && seenID[d.ID]) || (d.CategoryCode == "SW8" && seenStation[stationBase(d.PlaceName)]) {
			return
		}
		seenID[d.ID] = true
		if d.CategoryCode == "SW8" {
			seenStation[stationBase(d.PlaceName)] = true
		}
		out = append(out, d)
	}
	if needsStationBoost(q, main) {
		for _, d := range stations {
			if stationBase(d.PlaceName) == q+"역" {
				add(d)
			}
		}
		if isRegionQuery(main) {
			for _, d := range stations {
				if strings.HasPrefix(stationBase(d.PlaceName), q) {
					add(d)
				}
			}
		}
	}
	for _, d := range main.Documents {
		add(d)
	}
	if len(out) > placesMax {
		out = out[:placesMax]
	}
	return out
}
