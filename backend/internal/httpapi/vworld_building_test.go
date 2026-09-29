package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// 서울 위도에서 경도 0.0001° ≈ 8.8m, 위도 0.0001° ≈ 11.1m.
const bldLat, bldLon = 37.5000, 127.0000

// bldFeature 는 (lat, lon) 을 가운데로 한 변 2*half(도) 정사각형 건물 하나의 GeoJSON feature 다.
func bldFeature(name, detail string, lat, lon, half float64) string {
	ring := fmt.Sprintf("[[%[1]f,%[3]f],[%[2]f,%[3]f],[%[2]f,%[4]f],[%[1]f,%[4]f],[%[1]f,%[3]f]]",
		lon-half, lon+half, lat-half, lat+half)
	return fmt.Sprintf(`{"type":"Feature","geometry":{"type":"MultiPolygon","coordinates":[[%s]]},`+
		`"properties":{"buld_nm":%q,"buld_nm_dc":%q,"rd_nm":"가나로","buld_no":"1"}}`, ring, name, detail)
}

func vworldOK(features ...string) string {
	return `{"response":{"status":"OK","result":{"featureCollection":{"type":"FeatureCollection","features":[` +
		strings.Join(features, ",") + `]}}}}`
}

func vworldServer(t *testing.T, body string, gotQuery *url.Values) *Server {
	t.Helper()
	vw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotQuery != nil {
			*gotQuery = r.URL.Query()
		}
		io.WriteString(w, body)
	}))
	t.Cleanup(vw.Close)
	return &Server{VWorldKey: "vk", VWorldBase: vw.URL, HTTP: vw.Client(), Log: slog.New(slog.DiscardHandler)}
}

func TestBuildingAt(t *testing.T) {
	// 가운데 점을 품은 101동(벽까지 약 26m) · 101동과 겹치고 점은 안 품은 상가(벽까지 약 9m) · 동쪽 102동 · 서쪽 이름 없는 건물
	in101 := bldFeature("가나아파트", "101동", bldLat, bldLon, 0.0003)
	overlap := bldFeature("가나상가", "", bldLat, bldLon+0.0002, 0.0001)
	near102 := bldFeature("가나아파트", "102동", bldLat, bldLon+0.0005, 0.0001) // 벽까지 약 35m
	unnamedWest := bldFeature("", "", bldLat, bldLon-0.0003, 0.0001)      // 벽까지 약 18m
	cases := []struct {
		name, body, want string // want = "건물명|상세 건물명"
	}{
		{"겹친 건물 벽이 더 가까워도 품은 건물이 이긴다", vworldOK(overlap, in101), "가나아파트|101동"},
		{"품은 건물이 없으면 가장 가까운 건물", vworldOK(near102, bldFeature("다라빌딩", "", bldLat+0.0006, bldLon, 0.0001)), "가나아파트|102동"},
		{"상세 건물명이 없으면 건물명만", vworldOK(bldFeature("다라빌딩", "", bldLat, bldLon, 0.0002)), "다라빌딩|"},
		{"가장 가까운 건물에 이름이 없으면 빈 값", vworldOK(near102, unnamedWest), "|"},
		{"상세 건물명만 있으면 빈 값", vworldOK(bldFeature("", "1동", bldLat, bldLon, 0.0002)), "|"},
		{"건물이 없으면 빈 값", vworldOK(), "|"},
		{"NOT_FOUND 면 빈 값", `{"response":{"status":"NOT_FOUND"}}`, "|"},
		{"Polygon 형식도 읽는다", vworldOK(strings.Replace(strings.Replace(in101,
			`"MultiPolygon","coordinates":[[`, `"Polygon","coordinates":[`, 1), `]]},"properties"`, `]},"properties"`, 1)),
			"가나아파트|101동"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := vworldServer(t, c.body, nil)
			if name, detail := s.buildingAt(context.Background(), bldLat, bldLon); name+"|"+detail != c.want {
				t.Errorf("got %q|%q, want %q", name, detail, c.want)
			}
		})
	}
}

// VWorld 는 POINT(경도 위도) 순서다. 바꿔 보내면 엉뚱한 곳의 건물이 온다.
func TestBuildingAtRequest(t *testing.T) {
	var q url.Values
	s := vworldServer(t, vworldOK(), &q)
	s.buildingAt(context.Background(), 37.5012345, 127.0012345)
	want := map[string]string{"data": "LT_C_SPBD", "key": "vk", "crs": "EPSG:4326", "geometry": "true",
		"geomFilter": "POINT(127.0012345 37.5012345)", "buffer": "30"}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s=%q, want %q", k, q.Get(k), v)
		}
	}
}

// 키가 없으면 VWorld 를 부르지 않고, 오류는 키·좌표 없이 원인만 로그에 남긴다.
func TestBuildingAtFailures(t *testing.T) {
	called := false
	vw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		io.WriteString(w, `{"response":{"status":"ERROR","error":{"level":"2","code":"INVALID_KEY",`+
			`"text":"등록되지 않은 인증키입니다."}}}`)
	}))
	defer vw.Close()
	noKey := &Server{VWorldBase: vw.URL, HTTP: vw.Client(), Log: slog.New(slog.DiscardHandler)}
	if name, _ := noKey.buildingAt(context.Background(), bldLat, bldLon); name != "" || called {
		t.Errorf("키 없음: got %q called=%v", name, called)
	}

	var buf bytes.Buffer
	s := &Server{VWorldKey: "secretkey", VWorldBase: vw.URL, HTTP: vw.Client(),
		Log: slog.New(slog.NewTextHandler(&buf, nil))}
	if name, _ := s.buildingAt(context.Background(), bldLat, bldLon); name != "" {
		t.Errorf("ERROR 응답: got %q", name)
	}
	if !strings.Contains(buf.String(), "INVALID_KEY") {
		t.Errorf("오류 코드가 로그에 없다: %s", buf.String())
	}

	buf.Reset()
	down := &Server{VWorldKey: "secretkey", VWorldBase: "http://127.0.0.1:9", HTTP: &http.Client{},
		Log: slog.New(slog.NewTextHandler(&buf, nil))}
	if name, _ := down.buildingAt(context.Background(), 37.5662952, 126.9779692); name != "" {
		t.Errorf("연결 실패: got %q", name)
	}
	logged := buf.String()
	if !strings.Contains(logged, "vworld building") || strings.Contains(logged, "secretkey") ||
		strings.Contains(logged, "37.566") || strings.Contains(logged, "126.977") {
		t.Errorf("로그: %s", logged)
	}
}
