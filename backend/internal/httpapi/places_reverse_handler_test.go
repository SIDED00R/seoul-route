package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 좌표 → 이름·주소. 이름은 건물 이름 > 도로명 주소 > 지번 주소 순으로 고른다(카카오는 건물 이름이 빈 좌표가 흔하다).
func TestPlacesReverse(t *testing.T) {
	var gotQuery string
	kakao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.Header.Get("Authorization") != "KakaoAK k" {
			t.Errorf("Authorization=%q", r.Header.Get("Authorization"))
		}
		io.WriteString(w, `{"documents":[{"road_address":{"building_name":"서울역","address_name":"서울 중구 세종대로 2"},
			"address":{"address_name":"서울 중구 남대문로5가 73"}}]}`)
	}))
	defer kakao.Close()
	s := &Server{KakaoKey: "k", KakaoBase: kakao.URL, HTTP: kakao.Client(), Log: slog.New(slog.DiscardHandler)}

	rr := httptest.NewRecorder()
	s.handlePlacesReverse(rr, httptest.NewRequest(http.MethodGet, "/places/reverse?lat=37.5547&lon=126.9707", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body)
	}
	var out map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["name"] != "서울역" || out["address"] != "서울 중구 세종대로 2" {
		t.Errorf("out=%v", out)
	}
	// 카카오는 x=경도, y=위도다. 바꿔 보내면 엉뚱한 곳의 주소가 온다.
	if !strings.Contains(gotQuery, "x=126.97") || !strings.Contains(gotQuery, "y=37.55") {
		t.Errorf("카카오 요청 쿼리=%q", gotQuery)
	}
}

func TestPlacesReverseFallbacks(t *testing.T) {
	cases := []struct {
		name, body, wantName, wantAddr string
	}{
		{"건물 이름 없으면 도로명 주소",
			`{"documents":[{"road_address":{"building_name":"","address_name":"서울 중구 세종대로 2"},
			 "address":{"address_name":"서울 중구 남대문로5가 73"}}]}`,
			"서울 중구 세종대로 2", "서울 중구 세종대로 2"},
		{"도로명 주소가 없으면 지번 주소",
			`{"documents":[{"road_address":null,"address":{"address_name":"서울 중구 남대문로5가 73"}}]}`,
			"서울 중구 남대문로5가 73", "서울 중구 남대문로5가 73"},
		{"결과가 없으면 빈 값", `{"documents":[]}`, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kakao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, c.body)
			}))
			defer kakao.Close()
			s := &Server{KakaoKey: "k", KakaoBase: kakao.URL, HTTP: kakao.Client(), Log: slog.New(slog.DiscardHandler)}
			rr := httptest.NewRecorder()
			s.handlePlacesReverse(rr, httptest.NewRequest(http.MethodGet, "/places/reverse?lat=37.55&lon=126.97", nil))
			var out map[string]string
			json.Unmarshal(rr.Body.Bytes(), &out)
			if out["name"] != c.wantName || out["address"] != c.wantAddr {
				t.Errorf("out=%v, want name=%q address=%q", out, c.wantName, c.wantAddr)
			}
		})
	}
}

// 카카오 호출이 실패해도 좌표는 로그에 남지 않는다(요청 URL 이 실린 url.Error 껍질을 벗긴다).
func TestPlacesReverseErrorLogHasNoCoords(t *testing.T) {
	var buf bytes.Buffer
	s := &Server{KakaoKey: "k", KakaoBase: "http://127.0.0.1:9", HTTP: &http.Client{},
		Log: slog.New(slog.NewTextHandler(&buf, nil))}
	rr := httptest.NewRecorder()
	s.handlePlacesReverse(rr, httptest.NewRequest(http.MethodGet, "/places/reverse?lat=37.5662952&lon=126.9779692", nil))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("code=%d", rr.Code)
	}
	logged := buf.String()
	if strings.Contains(logged, "37.566") || strings.Contains(logged, "126.977") {
		t.Errorf("로그에 좌표가 남았다: %s", logged)
	}
	if !strings.Contains(logged, "kakao coord2address") {
		t.Errorf("실패 원인이 로그에 없다: %s", logged)
	}
}

func TestPlacesReverseErrors(t *testing.T) {
	kakao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer kakao.Close()
	log := slog.New(slog.DiscardHandler)

	// 키가 없으면 503(앱은 이름 없이 "현재 위치" 로 보여 준다)
	noKey := &Server{HTTP: kakao.Client(), Log: log}
	rr := httptest.NewRecorder()
	noKey.handlePlacesReverse(rr, httptest.NewRequest(http.MethodGet, "/places/reverse?lat=37.55&lon=126.97", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("키 없음 code=%d", rr.Code)
	}

	s := &Server{KakaoKey: "k", KakaoBase: kakao.URL, HTTP: kakao.Client(), Log: log}
	for _, q := range []string{"", "?lat=37.55", "?lat=abc&lon=126.97", "?lat=35.1&lon=129.0", "?lat=37.55&lon=0",
		"?lat=NaN&lon=126.97", "?lat=37.55&lon=nan"} {
		rr := httptest.NewRecorder()
		s.handlePlacesReverse(rr, httptest.NewRequest(http.MethodGet, "/places/reverse"+q, nil))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("좌표 %q code=%d", q, rr.Code)
		}
	}
	// 카카오가 오류면 502
	rr = httptest.NewRecorder()
	s.handlePlacesReverse(rr, httptest.NewRequest(http.MethodGet, "/places/reverse?lat=37.55&lon=126.97", nil))
	if rr.Code != http.StatusBadGateway {
		t.Errorf("카카오 401 → code=%d", rr.Code)
	}
}
