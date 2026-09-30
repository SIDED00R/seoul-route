package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	// 카카오는 x=경도, y=위도다.
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

// 카카오 건물 이름은 단지 대표 동이다. 반경 안 같은 단지의 가장 가까운 VWorld 동으로 바꾸고, 주소는 카카오 그대로다.
// 이름이 다른 건물(카카오 자체 이름)은 카카오 이름을 두고, 카카오 이름이 없을 때만 VWorld 이름을 쓴다.
func TestPlacesReverseBuildingName(t *testing.T) {
	const withName = `{"documents":[{"road_address":{"building_name":"가나아파트 105동","address_name":"서울 가나구 가나로 1"}}]}`
	const noName = `{"documents":[{"road_address":{"building_name":"","address_name":"서울 가나구 가나로 1"}}]}`
	house := bldFeature("", "", bldLat, bldLon+0.00012, 0.00005)               // 서쪽 벽까지 약 6m
	complexShop := bldFeature("가나아파트", "상가동", bldLat, bldLon-0.00018, 0.00005) // 동쪽 벽까지 약 11m
	cases := []struct {
		name, kakao, vworld, want string
	}{
		{"같은 단지면 VWorld 동", withName, vworldOK(bldFeature("가나아파트", "101동", bldLat, bldLon, 0.0003)), "가나아파트 101동"},
		{"VWorld 에 건물이 없으면 카카오", withName, vworldOK(), "가나아파트 105동"},
		{"VWorld 오류면 카카오", withName, `{"response":{"status":"ERROR","error":{"code":"INVALID_KEY"}}}`, "가나아파트 105동"},
		{"이름이 다른 건물이면 카카오", withName, vworldOK(bldFeature("가나종합단지", "전시동", bldLat, bldLon, 0.0003)), "가나아파트 105동"},
		{"같은 단지라도 동이 없으면 카카오", withName, vworldOK(bldFeature("가나아파트", "", bldLat, bldLon, 0.0003)), "가나아파트 105동"},
		{"카카오 이름이 없으면 VWorld 건물", noName, vworldOK(bldFeature("가나빌딩", "", bldLat, bldLon, 0.0003)), "가나빌딩"},
		{"카카오 이름이 없으면 VWorld 건물과 동", noName, vworldOK(bldFeature("가나종합단지", "전시동", bldLat, bldLon, 0.0003)), "가나종합단지 전시동"},
		{"둘 다 없으면 주소", noName, vworldOK(), "서울 가나구 가나로 1"},
		// 단지 가장자리: 가장 가까운 건물은 단지 밖 이름 없는 집(약 6m), 같은 단지 상가동은 약 11m
		{"가장 가까운 건물이 이름 없는 집이어도 같은 단지 동", withName, vworldOK(house, complexShop), "가나아파트 상가동"},
		{"다른 단지 동이 더 가까워도 같은 단지 동", withName,
			vworldOK(bldFeature("다라아파트", "201동", bldLat, bldLon+0.00012, 0.00005), complexShop), "가나아파트 상가동"},
		{"카카오 이름이 없으면 가장 가까운 건물만 본다(이름 없으면 주소)", noName, vworldOK(house, complexShop), "서울 가나구 가나로 1"},
		{"카카오 이름이 없고 가장 가까운 건물이 동 이름만 있으면 주소", noName,
			vworldOK(bldFeature("", "1동", bldLat, bldLon, 0.0003)), "서울 가나구 가나로 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kakao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, c.kakao)
			}))
			defer kakao.Close()
			s := vworldServer(t, c.vworld, nil)
			s.KakaoKey, s.KakaoBase = "k", kakao.URL
			rr := httptest.NewRecorder()
			s.handlePlacesReverse(rr, httptest.NewRequest(http.MethodGet,
				fmt.Sprintf("/places/reverse?lat=%f&lon=%f", bldLat, bldLon), nil))
			var out map[string]string
			json.Unmarshal(rr.Body.Bytes(), &out)
			if rr.Code != http.StatusOK || out["name"] != c.want || out["address"] != "서울 가나구 가나로 1" {
				t.Errorf("code=%d out=%v, want name %q", rr.Code, out, c.want)
			}
		})
	}
}

// VWorld 가 답하지 않으면 buildingTimeout 뒤 카카오 이름·주소로 답하고, 로그에는 원인만 남는다.
func TestPlacesReverseSlowVWorld(t *testing.T) {
	defer func(d time.Duration) { buildingTimeout = d }(buildingTimeout)
	buildingTimeout = 50 * time.Millisecond
	kakao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"documents":[{"road_address":{"building_name":"가나아파트 105동","address_name":"서울 가나구 가나로 1"}}]}`)
	}))
	defer kakao.Close()
	vw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer vw.Close()
	var buf bytes.Buffer
	s := &Server{KakaoKey: "k", KakaoBase: kakao.URL, VWorldKey: "secretkey", VWorldBase: vw.URL, HTTP: &http.Client{},
		Log: slog.New(slog.NewTextHandler(&buf, nil))}
	// 제한이 빠지면 무기한 멈추지 않고 2초에 끝나 실패하도록 요청에도 상한을 건다.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	rr := httptest.NewRecorder()
	s.handlePlacesReverse(rr, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/places/reverse?lat=%f&lon=%f", bldLat, bldLon), nil).WithContext(ctx))
	elapsed := time.Since(start)
	var out map[string]string
	json.Unmarshal(rr.Body.Bytes(), &out)
	if rr.Code != http.StatusOK || out["name"] != "가나아파트 105동" || out["address"] != "서울 가나구 가나로 1" ||
		elapsed > time.Second {
		t.Errorf("code=%d out=%v elapsed=%v", rr.Code, out, elapsed)
	}
	if logged := buf.String(); !strings.Contains(logged, "deadline exceeded") || strings.Contains(logged, "secretkey") {
		t.Errorf("로그: %s", logged)
	}
}

// 즐겨찾기 50m 안이면 즐겨찾기 장소 이름이 건물 이름보다 앞선다. 남의 즐겨찾기는 보지 않는다.
func TestPlacesReverseFavoriteName(t *testing.T) {
	s, pool := testServer(t)
	kakao := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"documents":[{"road_address":{"building_name":"가나아파트 105동","address_name":"서울 가나구 가나로 1"}}]}`)
	}))
	defer kakao.Close()
	vw := vworldServer(t, vworldOK(bldFeature("가나아파트", "101동", bldLat, bldLon, 0.0003)), nil)
	s.KakaoKey, s.KakaoBase, s.VWorldKey, s.VWorldBase = "k", kakao.URL, vw.VWorldKey, vw.VWorldBase
	h := s.Router()
	login := func(sub string) string {
		t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = $1`, sub) })
		_, out := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, "")
		return out["token"].(string)
	}
	stamp := time.Now().Format("150405.000000")
	me, other := login("sub-revfav-me-"+stamp), login("sub-revfav-other-"+stamp)
	// 즐겨찾기는 요청점에서 북쪽으로 위도 0.0004°(약 44m)
	fav := fmt.Sprintf(`{"kind":"home","label":"집","place":{"name":"가나아파트 102동","lat":%f,"lon":%f}}`,
		bldLat+0.0004, bldLon)
	if rr, _ := do(t, h, http.MethodPost, "/users/me/favorites", fav, me); rr.Code != http.StatusCreated {
		t.Fatalf("즐겨찾기 code=%d", rr.Code)
	}
	cases := []struct {
		name, token string
		lat         float64
		want        string
	}{
		{"44m 면 즐겨찾기", me, bldLat, "가나아파트 102동"},
		{"55m 면 건물", me, bldLat - 0.0001, "가나아파트 101동"},
		{"남의 즐겨찾기는 안 본다", other, bldLat, "가나아파트 101동"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr, out := do(t, h, http.MethodGet, fmt.Sprintf("/places/reverse?lat=%f&lon=%f", c.lat, bldLon), "", c.token)
			if rr.Code != http.StatusOK || out["name"] != c.want || out["address"] != "서울 가나구 가나로 1" {
				t.Errorf("code=%d out=%v, want name %q", rr.Code, out, c.want)
			}
		})
	}
}
