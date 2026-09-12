package build

import (
	"testing"

	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
)

// 같은 기준명이 800m 안이면 한 역, 밖(동명이역 양평 53km)이면 다른 역. 부모 좌표는 자식 평균.
func TestStationGroups(t *testing.T) {
	stops := []ktdb.Row{
		{"stop_id": "RS_1", "stop_name": "서울(1호선)", "stop_lat": "37.5546", "stop_lon": "126.9722"},
		{"stop_id": "RS_2", "stop_name": "서울(4호선)", "stop_lat": "37.5530", "stop_lon": "126.9720"},
		{"stop_id": "RS_3", "stop_name": "양평(5호선)", "stop_lat": "37.5253", "stop_lon": "126.8855"},
		{"stop_id": "RS_4", "stop_name": "양평(경의중앙선)", "stop_lat": "37.4925", "stop_lon": "127.4907"},
		{"stop_id": "RS_5", "stop_name": "강남", "stop_lat": "37.4979", "stop_lon": "127.0276"},
	}
	parents, parentOf := stationGroups(stops)
	if len(parents) != 4 {
		t.Fatalf("부모역 4개(서울·양평·양평_2·강남)여야: %v", parents)
	}
	if parentOf["RS_1"] != "ST_서울" || parentOf["RS_2"] != "ST_서울" {
		t.Fatalf("서울 1·4호선은 같은 역: %v", parentOf)
	}
	if parentOf["RS_3"] == parentOf["RS_4"] {
		t.Fatalf("53km 떨어진 양평은 다른 역이어야: %v", parentOf)
	}
	for _, p := range parents {
		if p[0] == "ST_서울" && (p[2] != "37.553800" || p[3] != "126.972100") {
			t.Fatalf("부모 좌표는 자식 평균: %v", p)
		}
	}
	// 입력 순서를 뒤집어도 동명이역의 접미사 배정이 같아야 한다(ktdb.Load 는 맵 순회라 순서가 매번 다르다).
	rev := []ktdb.Row{stops[4], stops[3], stops[2], stops[1], stops[0]}
	parents2, parentOf2 := stationGroups(rev)
	if parentOf2["RS_3"] != parentOf["RS_3"] || parentOf2["RS_4"] != parentOf["RS_4"] {
		t.Fatalf("입력 순서에 따라 양평 ID 가 뒤바뀜: %v vs %v", parentOf, parentOf2)
	}
	for i := range parents {
		if parents[i][0] != parents2[i][0] || parents[i][2] != parents2[i][2] {
			t.Fatalf("부모 행이 입력 순서에 의존: %v vs %v", parents[i], parents2[i])
		}
	}
}
