// Package shops 는 서울 상가 목록(otp/fetch_shop_places.py 산출)을 메모리에 올려 이름 부분 일치로 찾는다.
// 카카오 키워드 검색이 찾지 못하는 입력 중인 단어("무교동북" → 무교동북어국집, "스타벅" → 스타벅스)를 메운다.
package shops

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/SIDED00R/seoul-route/backend/internal/geo"
)

// MinQueryRunes: 부분 일치를 찾는 검색어의 최소 글자 수(공백 제외). 한 글자는 수만 곳에 걸린다.
const MinQueryRunes = 2

// Shop 은 상가 한 곳이다. Name 은 상호명과 지점명을 이은 이름이다("스타벅스 무교로점").
type Shop struct {
	ID       string
	Name     string
	Category string // 상권업종 소분류명(예: "백반/한정식")
	Address  string // 도로명주소, 없으면 지번주소
	Lat, Lon float64
	key      string // Key(Name): 검색 대상
}

// Index 는 상가 목록이다.
type Index struct{ shops []Shop }

// Len 은 담긴 상가 수.
func (x *Index) Len() int { return len(x.shops) }

// Key 는 비교용 이름이다: 공백을 없애고 소문자로.
func Key(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), "")) }

// New 는 상가 목록으로 색인을 만든다.
func New(list []Shop) *Index {
	x := &Index{shops: make([]Shop, len(list))}
	for i, s := range list {
		s.key = Key(s.Name)
		x.shops[i] = s
	}
	return x
}

// Load 는 shop-places-seoul.csv(id,name,branch,category,address,lon,lat)를 읽는다.
func Load(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.ReuseRecord = true
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: 헤더 없음: %w", path, err)
	}
	want := []string{"id", "name", "branch", "category", "address", "lon", "lat"}
	if strings.Join(header, ",") != strings.Join(want, ",") {
		return nil, fmt.Errorf("%s: 헤더 %v, 기대 %v", path, header, want)
	}
	var list []Shop
	for line := 2; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		lon, errX := strconv.ParseFloat(rec[5], 64)
		lat, errY := strconv.ParseFloat(rec[6], 64)
		if errX != nil || errY != nil {
			return nil, fmt.Errorf("%s:%d: 좌표 %q,%q", path, line, rec[5], rec[6])
		}
		name := rec[1]
		if rec[2] != "" {
			name += " " + rec[2]
		}
		list = append(list, Shop{ID: rec[0], Name: name, Category: rec[3], Address: rec[4], Lat: lat, Lon: lon})
	}
	return New(list), nil
}

// Match 는 검색 결과 한 곳과 기준 위치에서의 직선거리(m, 위치가 없으면 0)다.
type Match struct {
	Shop
	DistanceM float64
}

// Search 는 이름(공백·대소문자 무시)이 q 를 품은 상가를 최대 limit 곳 돌려준다. 이름이 q 로 시작하는 곳이 먼저이고,
// 그 안에서 located 면 (lat, lon) 에서 가까운 순, 아니면 이름이 짧은 순이다. q 가 MinQueryRunes 글자보다 짧으면 nil.
func (x *Index) Search(q string, lat, lon float64, located bool, limit int) []Match {
	k := Key(q)
	if utf8.RuneCountInString(k) < MinQueryRunes {
		return nil
	}
	type hit struct {
		i      int
		prefix bool
		d      float64
	}
	var hits []hit
	for i := range x.shops {
		s := &x.shops[i]
		if !strings.Contains(s.key, k) {
			continue
		}
		h := hit{i: i, prefix: strings.HasPrefix(s.key, k)}
		if located {
			h.d = geo.DistM(lat, lon, s.Lat, s.Lon)
		}
		hits = append(hits, h)
	}
	sort.Slice(hits, func(a, b int) bool {
		ha, hb := hits[a], hits[b]
		if ha.prefix != hb.prefix {
			return ha.prefix
		}
		if located && ha.d != hb.d {
			return ha.d < hb.d
		}
		if la, lb := len(x.shops[ha.i].key), len(x.shops[hb.i].key); la != lb {
			return la < lb
		}
		return ha.i < hb.i
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Match, len(hits))
	for j, h := range hits {
		out[j] = Match{Shop: x.shops[h.i], DistanceM: h.d}
	}
	return out
}
