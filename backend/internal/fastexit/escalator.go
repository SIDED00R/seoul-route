package fastexit

import (
	"encoding/json"
	"os"
	"strings"
)

// EscalatorAPIURL 은 서울 열린데이터광장 「서울교통공사_에스컬레이터 설치현황」. 인증키는 SEOUL_OPENAPI_KEY 를 쓴다.
// 키가 URL 경로에 들어가므로 오류에 URL 을 싣지 않는다.
const EscalatorAPIURL = "http://openapi.seoul.go.kr:8088"

// EscalatorService 는 그 데이터셋의 서비스명.
const EscalatorService = "tbTrfcEscalInstlPrst"

// Escalator 는 승강기번호로 빠른하차 자료와 연결되는 에스컬레이터다.
type Escalator struct {
	No        string `json:"ESCAL_NO"`
	Direction string `json:"DIRECTION"`  // 상행·하행·상/하겸용
	Section   string `json:"SECTION"`    // 운행구간 "B2-B1"
	Position  string `json:"INSTL_PSTN"` // 설치위치 "1번 출입구", "환승통로(1,3호선)"
}

// goesUp 은 승강장에서 위로 올라가는 에스컬레이터인지. 내릴 때 쓸모가 있는 건 이쪽뿐이다.
func (e Escalator) goesUp() bool { return e.Direction != "하행" }

// transfer 는 이 에스컬레이터가 환승 통로로 이어지는지.
func (e Escalator) transfer() bool { return strings.Contains(e.Position, "환승") }

// LoadEscalators 는 cmd/fastexit 이 저장한 JSON(행 배열)을 승강기번호 → 에스컬레이터로 읽는다.
func LoadEscalators(path string) (map[string]Escalator, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []Escalator
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	out := make(map[string]Escalator, len(rows))
	for _, r := range rows {
		if no := strings.TrimSpace(r.No); no != "" {
			out[no] = r
		}
	}
	return out, nil
}
