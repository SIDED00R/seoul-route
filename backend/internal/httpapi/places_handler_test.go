package httpapi

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
