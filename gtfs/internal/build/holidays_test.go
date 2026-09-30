package build

import (
	"strings"
	"testing"
)

// datesOf 는 날짜 d 의 calendar_dates 행을 "service:exception" 으로 모은다.
func datesOf(rows [][]string, d string) string {
	var out []string
	for _, r := range rows {
		if r[1] == d {
			out = append(out, r[0]+":"+r[2])
		}
	}
	return strings.Join(out, ",")
}

func TestHolidayCalendarDates(t *testing.T) {
	rows := holidayCalendarDates(true)
	cases := []struct{ name, date, want string }{
		{"평일 공휴일(개천절 대체, 월): 평일 빼고 휴일·토일 시각표", "20261005", "WEEKDAY:2,SUN:1,SATSUN:1"},
		{"토요일 공휴일(개천절): 토요일 빼고 휴일 시각표", "20261003", "SAT:2,SUN:1"},
		{"일요일 공휴일(삼일절)은 그대로", "20260301", ""},
		{"공휴일이 아닌 날은 없음", "20261006", ""},
	}
	for _, c := range cases {
		if got := datesOf(rows, c.date); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if got := datesOf(holidayCalendarDates(false), "20261005"); got != "WEEKDAY:2,SUN:1" {
		t.Errorf("SATSUN 이 없으면 넣지 않는다: %q", got)
	}
}

// 목록은 날짜 형식이 맞고 겹치지 않으며 정렬돼 있다.
func TestKoreanHolidaysWellFormed(t *testing.T) {
	seen := map[string]bool{}
	prev := ""
	for _, d := range koreanHolidays {
		if len(d) != 8 || d <= prev || seen[d] {
			t.Errorf("잘못된 항목 %q (앞 %q)", d, prev)
		}
		seen[d] = true
		prev = d
	}
}
