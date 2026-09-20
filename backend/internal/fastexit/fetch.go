package fastexit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// APIURL 은 공공데이터포털 「서울교통공사_빠른하차정보」 조회 주소.
const APIURL = "https://apis.data.go.kr/B553766/inout/getFstExit"

// pageSize 는 API 한 번에 받을 행 수다.
const pageSize = 1000

type page struct {
	Response struct {
		Header struct {
			Code string `json:"resultCode"`
			Msg  string `json:"resultMsg"`
		} `json:"header"`
		Body struct {
			Items struct {
				Item []json.RawMessage `json:"item"`
			} `json:"items"`
			Total int `json:"totalCount"`
		} `json:"body"`
	} `json:"response"`
}

// FetchEscalators 는 에스컬레이터 설치현황 전체를 API 응답 그대로 모아 돌려준다. 열린데이터광장은 1콜 1,000행이다.
// 키는 URL 경로에 들어가므로 오류에 URL 을 싣지 않는다.
func FetchEscalators(hc *http.Client, base, key string) ([]json.RawMessage, error) {
	var out []json.RawMessage
	for start := 1; ; start += pageSize {
		u := fmt.Sprintf("%s/%s/json/%s/%d/%d/", base, url.PathEscape(key), EscalatorService, start, start+pageSize-1)
		resp, err := hc.Get(u)
		if err != nil {
			var ue *url.Error
			if errors.As(err, &ue) {
				err = ue.Err
			}
			return nil, fmt.Errorf("에스컬레이터 API %d행부터: %w", start, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("에스컬레이터 API %d행부터: %w", start, err)
		}
		// 오류는 HTTP 200 으로 오고 봉투가 세 가지다: 서비스 래퍼 안(RESULT), 래퍼 없는 최상위 RESULT, 평면 CODE.
		// 래퍼만 보면 최상위 오류가 빈 코드로 통과해 0행이 정상 결과가 된다(gbfs/poller.go 가 같은 호스트에서 셋을
		// 모두 본다). 잘못된 키는 /json/ 요청에도 XML 로 오므로 위 Unmarshal 이 막는다.
		var p struct {
			Result struct {
				Code    string `json:"CODE"`
				Message string `json:"MESSAGE"`
			} `json:"RESULT"`
			Code    string `json:"CODE"`
			Message string `json:"MESSAGE"`
			Body    struct {
				Result struct {
					Code    string `json:"CODE"`
					Message string `json:"MESSAGE"`
				} `json:"RESULT"`
				Row []json.RawMessage `json:"row"`
			} `json:"tbTrfcEscalInstlPrst"`
		}
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, fmt.Errorf("에스컬레이터 API %d행부터: HTTP %d, JSON 아님: %.120s", start, resp.StatusCode, body)
		}
		for _, r := range [...]struct{ code, message string }{
			{p.Result.Code, p.Result.Message}, {p.Code, p.Message}, {p.Body.Result.Code, p.Body.Result.Message},
		} {
			switch r.code {
			case "", "INFO-000":
			case "INFO-200": // 범위 밖 = 빈 페이지. 모아 둔 행을 그대로 돌려준다(전체가 1,000의 배수일 때 온다)
				return out, nil
			default:
				return nil, fmt.Errorf("에스컬레이터 API %d행부터: %s %s", start, r.code, r.message)
			}
		}
		out = append(out, p.Body.Row...)
		if len(p.Body.Row) < pageSize {
			return out, nil
		}
	}
}

// Fetch 는 전체 행을 API 응답 그대로 모아 돌려준다. 키는 요청 URL 에만 들어가므로 오류에는 URL 을 싣지 않는다.
func Fetch(hc *http.Client, base, key string) ([]json.RawMessage, error) {
	var out []json.RawMessage
	for pageNo := 1; ; pageNo++ {
		q := url.Values{"serviceKey": {key}, "dataType": {"JSON"}, "pageNo": {strconv.Itoa(pageNo)},
			"numOfRows": {strconv.Itoa(pageSize)}}
		resp, err := hc.Get(base + "?" + q.Encode())
		if err != nil {
			var ue *url.Error
			if errors.As(err, &ue) {
				err = ue.Err
			}
			return nil, fmt.Errorf("빠른하차 API %d쪽: %w", pageNo, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("빠른하차 API %d쪽: %w", pageNo, err)
		}
		var p page
		if err := json.Unmarshal(body, &p); err != nil { // 키 오류·한도 초과는 XML 로 온다
			return nil, fmt.Errorf("빠른하차 API %d쪽: HTTP %d, JSON 아님: %.120s", pageNo, resp.StatusCode, body)
		}
		if p.Response.Header.Code != "00" {
			return nil, fmt.Errorf("빠른하차 API %d쪽: %s %s", pageNo, p.Response.Header.Code, p.Response.Header.Msg)
		}
		out = append(out, p.Response.Body.Items.Item...)
		if len(p.Response.Body.Items.Item) == 0 || len(out) >= p.Response.Body.Total {
			return out, nil
		}
	}
}
