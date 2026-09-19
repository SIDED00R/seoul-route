package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 검색 결과는 서울 bbox 로 한정해 요청하고, 주소는 도로명 > 지번 순으로 고른다.
func TestPlacesSearch(t *testing.T) {
	var gotQuery string
	kakao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.Header.Get("Authorization") != "KakaoAK k" {
			t.Errorf("Authorization=%q", r.Header.Get("Authorization"))
		}
		io.WriteString(w, `{"documents":[
			{"place_name":"서울역","road_address_name":"서울 중구 세종대로 2","address_name":"서울 중구 남대문로5가 73",
			 "category_group_name":"지하철역","x":"126.9707","y":"37.5547"},
			{"place_name":"주소만","road_address_name":"","address_name":"서울 중구 어딘가","x":"127.0","y":"37.5"},
			{"place_name":"좌표 깨짐","address_name":"서울","x":"x","y":"y"}]}`)
	}))
	defer kakao.Close()
	s := &Server{KakaoKey: "k", KakaoBase: kakao.URL, HTTP: kakao.Client(), Log: slog.New(slog.DiscardHandler)}

	rr := httptest.NewRecorder()
	s.handlePlacesSearch(rr, httptest.NewRequest(http.MethodGet, "/places/search?q=서울역", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body)
	}
	if !strings.Contains(gotQuery, "rect=") || !strings.Contains(gotQuery, "size=10") {
		t.Errorf("서울 bbox·개수 제한이 빠졌다: %s", gotQuery)
	}
	var out struct {
		Places []Place `json:"places"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("응답 파싱: %v", err)
	}
	if len(out.Places) != 2 { // 좌표를 못 읽은 행은 버린다
		t.Fatalf("places=%d (기대 2)", len(out.Places))
	}
	if out.Places[0].Name != "서울역" || out.Places[0].Address != "서울 중구 세종대로 2" ||
		out.Places[0].Category != "지하철역" || out.Places[0].Lat != 37.5547 || out.Places[0].Lon != 126.9707 {
		t.Errorf("첫 결과: %+v", out.Places[0])
	}
	if out.Places[1].Address != "서울 중구 어딘가" { // 도로명이 비면 지번
		t.Errorf("둘째 결과 주소: %q", out.Places[1].Address)
	}
}

// 카카오에 닿지 못하는 상황(DNS·연결 실패)을 흉내 낸다. http.Client 는 이 오류를 url.Error 로 감싸며,
// 그 문자열에는 검색어가 든 요청 URL 이 들어간다.
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("dial tcp: lookup dapi.kakao.com: no such host")
}

// 카카오 호출이 실패해도 검색어는 로그에 남지 않는다.
func TestPlacesSearchErrorLogHasNoQuery(t *testing.T) {
	var buf bytes.Buffer
	s := &Server{KakaoKey: "k", HTTP: &http.Client{Transport: failingTransport{}},
		Log: slog.New(slog.NewTextHandler(&buf, nil))}
	rr := httptest.NewRecorder()
	s.handlePlacesSearch(rr,
		httptest.NewRequest(http.MethodGet, "/places/search?q=%EB%B9%84%EB%B0%80%EC%9E%A5%EC%86%8C", nil))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body)
	}
	logged := buf.String()
	if strings.Contains(logged, "비밀장소") || strings.Contains(logged, "%EB%B9%84") {
		t.Errorf("로그에 검색어가 남았다: %s", logged)
	}
	if !strings.Contains(logged, "kakao search") {
		t.Errorf("실패 원인이 로그에 없다: %s", logged)
	}
}
