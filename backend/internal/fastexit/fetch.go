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

// pageSize: 한 번에 받는 행 수. 2026-09-19 전체 2,358행이 3회 호출로 끝났다(개발계정 하루 10,000건).
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
