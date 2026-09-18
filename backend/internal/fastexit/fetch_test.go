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
