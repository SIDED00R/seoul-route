package fastexit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pageBody(total int, items ...string) string {
	return fmt.Sprintf(`{"response":{"header":{"resultCode":"00","resultMsg":"NORMAL_CODE"},`+
		`"body":{"items":{"item":[%s]},"totalCount":%d}}}`, strings.Join(items, ","), total)
}

func TestFetchPages(t *testing.T) {
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("serviceKey") != "k+/=" || q.Get("dataType") != "JSON" || q.Get("numOfRows") != "1000" {
			t.Errorf("질의: %v", q)
		}
		pages = append(pages, q.Get("pageNo"))
		if q.Get("pageNo") == "1" {
			fmt.Fprint(w, pageBody(3, `{"stnNm":"사당"}`, `{"stnNm":"방배"}`))
			return
		}
		fmt.Fprint(w, pageBody(3, `{"stnNm":"서울역"}`))
	}))
	defer srv.Close()
	rows, err := Fetch(srv.Client(), srv.URL, "k+/=")
	if err != nil || len(rows) != 3 || string(rows[2]) != `{"stnNm":"서울역"}` {
		t.Fatalf("rows=%s err=%v", rows, err)
	}
	if strings.Join(pages, ",") != "1,2" {
		t.Errorf("호출한 쪽: %v", pages)
	}
}

func TestFetchErrors(t *testing.T) {
	body := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
	body = `<OpenAPI_ServiceResponse><returnAuthMsg>SERVICE_KEY_IS_NOT_REGISTERED_ERROR</returnAuthMsg>` +
		`</OpenAPI_ServiceResponse>`
	if _, err := Fetch(srv.Client(), srv.URL, "secret-key"); err == nil || !strings.Contains(err.Error(), "JSON 아님") {
		t.Errorf("XML 오류 응답: %v", err)
	}
	body = `{"response":{"header":{"resultCode":"22","resultMsg":"LIMITED"},"body":{}}}`
	if _, err := Fetch(srv.Client(), srv.URL, "secret-key"); err == nil || !strings.Contains(err.Error(), "22 LIMITED") {
		t.Errorf("결과 코드 오류: %v", err)
	}
	srv.Close()
	_, err := Fetch(srv.Client(), srv.URL, "secret-key") // 연결 실패
	if err == nil || strings.Contains(err.Error(), "secret-key") || strings.Contains(err.Error(), "serviceKey") {
		t.Errorf("연결 오류에 키·URL 이 남았다: %v", err)
	}
}

func escPage(rows ...string) string {
	return fmt.Sprintf(`{"tbTrfcEscalInstlPrst":{"RESULT":{"CODE":"INFO-000","MESSAGE":"정상 처리되었습니다"},`+
		`"row":[%s]}}`, strings.Join(rows, ","))
}

// 오류가 HTTP 200 으로 오는 봉투 세 가지를 전부 막는지. 래퍼 안쪽만 보면 최상위 오류가 빈 코드로 통과해 0행이
// 정상 결과가 된다 — 그러면 수집기가 기존 escalator.json 을 "null" 로 덮는다.
func TestFetchEscalatorsErrorEnvelopes(t *testing.T) {
	body := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
	defer srv.Close()
	for _, c := range []struct{ name, resp, want string }{
		{"최상위 RESULT", `{"RESULT":{"CODE":"ERROR-500","MESSAGE":"서버 오류입니다"}}`, "ERROR-500"},
		{"평면 CODE", `{"CODE":"ERROR-334","MESSAGE":"요청 범위 오류"}`, "ERROR-334"},
		{"래퍼 안쪽", `{"tbTrfcEscalInstlPrst":{"RESULT":{"CODE":"ERROR-600","MESSAGE":"DB 오류"}}}`, "ERROR-600"},
	} {
		body = c.resp
		rows, err := FetchEscalators(srv.Client(), srv.URL, "secret-key")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: rows=%d err=%v", c.name, len(rows), err)
		}
		if err != nil && strings.Contains(err.Error(), "secret-key") {
			t.Errorf("%s: 오류에 키가 남았다: %v", c.name, err)
		}
	}
}

// INFO-200 은 오류가 아니라 "범위 밖 = 빈 페이지" 다. 전체 행수가 정확히 1,000의 배수면 마지막 쪽에서 이걸 받는데,
// 오류로 처리하면 모아 둔 행 전량이 버려진다.
func TestFetchEscalatorsEmptyLastPage(t *testing.T) {
	var start []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start = append(start, r.URL.Path)
		if len(start) == 1 {
			rows := make([]string, pageSize)
			for i := range rows {
				rows[i] = `{"ESCAL_NO":"1"}`
			}
			fmt.Fprint(w, escPage(rows...))
			return
		}
		fmt.Fprint(w, `{"RESULT":{"CODE":"INFO-200","MESSAGE":"해당하는 데이터가 없습니다."}}`)
	}))
	defer srv.Close()
	rows, err := FetchEscalators(srv.Client(), srv.URL, "k")
	if err != nil || len(rows) != pageSize {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if len(start) != 2 {
		t.Errorf("호출 쪽수 %d", len(start))
	}
}
