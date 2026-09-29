package build

import "time"

// koreanHolidays: 관공서 공휴일(대체공휴일·선거일 포함), YYYYMMDD. 출처: 「공휴일에 관한 법률」·「관공서의 공휴일에 관한 규정」
// (2026-02 개정으로 노동절·제헌절 공휴일)과 python holidays 0.8x(KR) 목록을 대조(2026-09-29). 음력 공휴일·대체공휴일·선거일이
// 해마다 달라 목록으로 둔다. 매년 말 다음 해 목록을 더하고, 임시공휴일이 지정되면 그때 더한다.
var koreanHolidays = []string{
	"20260101", "20260216", "20260217", "20260218", "20260301", "20260302", "20260501", "20260505", "20260524",
	"20260525", "20260603", "20260606", "20260717", "20260815", "20260817", "20260924", "20260925", "20260926",
	"20261003", "20261005", "20261009", "20261225",
	"20270101", "20270206", "20270207", "20270208", "20270209", "20270301", "20270501", "20270503", "20270505",
	"20270513", "20270606", "20270717", "20270719", "20270815", "20270816", "20270914", "20270915", "20270916",
	"20271003", "20271004", "20271009", "20271011", "20271225", "20271227",
}

// holidayCalendarDates 는 요일별 시각표(WEEKDAY·SAT·SUN, satsun 이면 SATSUN 도)가 공휴일에 휴일 시각표로 다니게 하는
// calendar_dates 행(service_id, date, exception_type 1=추가·2=제외)이다. 서울교통공사 시각표 구분이 평일·토요일·
// "일요일 및 공휴일"이다. 평일 공휴일: WEEKDAY 제외, SUN·SATSUN 추가. 토요일 공휴일: SAT 제외, SUN 추가(SATSUN 은 이미
// 운행). 일요일 공휴일은 그대로. 서비스 기간(ServiceStart~ServiceEnd) 밖은 뺀다. 버스·파일럿(ALL)은 요일 구분이 없어 그대로다.
func holidayCalendarDates(satsun bool) [][]string {
	var rows [][]string
	for _, d := range koreanHolidays {
		if d < ServiceStart || d > ServiceEnd {
			continue
		}
		t, err := time.Parse("20060102", d)
		if err != nil {
			continue
		}
		switch t.Weekday() {
		case time.Sunday:
		case time.Saturday:
			rows = append(rows, []string{"SAT", d, "2"}, []string{"SUN", d, "1"})
		default:
			rows = append(rows, []string{"WEEKDAY", d, "2"}, []string{"SUN", d, "1"})
			if satsun {
				rows = append(rows, []string{"SATSUN", d, "1"})
			}
		}
	}
	return rows
}
