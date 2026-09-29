package httpapi

import (
	"fmt"
	"strings"
	"testing"
)

func doc(id, name, code string) kakaoPlaceDoc {
	return kakaoPlaceDoc{ID: id, PlaceName: name, CategoryCode: code, X: "127.0", Y: "37.5"}
}

func station(id, name string) kakaoPlaceDoc { return doc(id, name, "SW8") }

func regionResult(docs ...kakaoPlaceDoc) kakaoKeywordResult {
	var r kakaoKeywordResult
	r.Meta.SameName.SelectedRegion = "서울 가나구"
	r.Documents = docs
	return r
}

func keywordResult(q string, docs ...kakaoPlaceDoc) kakaoKeywordResult {
	var r kakaoKeywordResult
	r.Meta.SameName.Keyword = q
	r.Documents = docs
	return r
}

func names(docs []kakaoPlaceDoc) string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.PlaceName
	}
	return strings.Join(out, " / ")
}

func TestNeedsStationBoost(t *testing.T) {
	cases := []struct {
		name, q string
		main    kakaoKeywordResult
		want    bool
	}{
		{"지역 이름", "강남", regionResult(doc("1", "강남스타일동상", "")), true},
		{"1위가 검색어로 시작하지 않음", "선릉", keywordResult("선릉", doc("1", "서울선릉과정릉", "")), true},
		{"1위가 검색어로 시작", "강남구청", keywordResult("강남구청", doc("1", "강남구청", "PO3")), false},
		{"결과 없음", "가나", keywordResult("가나"), true},
		{"이미 역으로 끝남", "선릉역", keywordResult("선릉역", doc("1", "서울선릉과정릉", "")), false},
	}
	for _, c := range cases {
		if got := needsStationBoost(c.q, c.main); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestRankPlaces(t *testing.T) {
	gangnamStations := []kakaoPlaceDoc{station("s1", "강남역 2호선"), station("s2", "강남역 신분당선"),
		station("s3", "신논현역 9호선"), station("s4", "강남구청역 7호선"), station("s5", "강남구청역 수인분당선")}
	cases := []struct {
		name, q  string
		main     kakaoKeywordResult
		stations []kakaoPlaceDoc
		want     string
	}{
		{"지역 이름: 같은 이름 역 → 이름으로 시작하는 역 → 카카오 순서, 노선 중복은 하나", "강남",
			regionResult(doc("m1", "서울선릉과정릉", ""), doc("m2", "압구정로데오거리", "AT4")), gangnamStations,
			"강남역 2호선 / 강남구청역 7호선 / 서울선릉과정릉 / 압구정로데오거리"},
		{"지역 이름이 아니면 같은 이름 역만 올리고 카카오 목록의 같은 역은 뺀다", "선릉",
			keywordResult("선릉", doc("m1", "서울선릉과정릉", ""), station("m2", "선릉역 수인분당선"), doc("m3", "멘토즈 선릉역점", "")),
			[]kakaoPlaceDoc{station("s1", "선릉역 2호선"), station("s2", "선정릉역 9호선")},
			"선릉역 2호선 / 서울선릉과정릉 / 멘토즈 선릉역점"},
		{"1위가 검색어로 시작하면 카카오 순서", "강남구청",
			keywordResult("강남구청", doc("m1", "강남구청", "PO3"), station("m2", "강남구청역 7호선")), gangnamStations,
			"강남구청 / 강남구청역 7호선"},
		{"올린 역과 같은 장소는 카카오 목록에서 한 번만", "가나",
			regionResult(doc("m1", "가나공원", ""), doc("s1", "가나역 2호선", "")), []kakaoPlaceDoc{station("s1", "가나역 2호선")},
			"가나역 2호선 / 가나공원"},
		{"id 가 없으면 같은 장소로 보지 않는다", "가나",
			keywordResult("가나", doc("", "가나 하나", ""), doc("", "가나 둘", "")), nil,
			"가나 하나 / 가나 둘"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := names(rankPlaces(c.q, c.main, c.stations)); got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestRankPlacesCap(t *testing.T) {
	var main []kakaoPlaceDoc
	for i := 0; i < 15; i++ {
		main = append(main, doc(fmt.Sprint("m", i), fmt.Sprint("명소 ", i), ""))
	}
	got := rankPlaces("가나", regionResult(main...), []kakaoPlaceDoc{station("s1", "가나역 2호선")})
	if len(got) != placesMax || got[0].PlaceName != "가나역 2호선" || got[placesMax-1].PlaceName != "명소 13" {
		t.Errorf("len=%d first=%q last=%q", len(got), got[0].PlaceName, got[len(got)-1].PlaceName)
	}
}
