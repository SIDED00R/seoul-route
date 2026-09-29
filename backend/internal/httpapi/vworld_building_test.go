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

// nearNames 는 buildingsNear 결과를 "건물명|상세 건물명" 을 " / " 로 이은 문자열로 적는다.
func nearNames(near []nearBuilding) string {
	out := make([]string, len(near))
	for i, b := range near {
		out[i] = b.name + "|" + b.detail
	}
	return strings.Join(out, " / ")
}

func TestBuildingsNear(t *testing.T) {
	// 가운데 점을 품은 101동(벽까지 약 26m) · 101동과 겹치고 점은 안 품은 상가(벽까지 약 9m) · 동쪽 102동 · 서쪽 이름 없는 건물
	in101 := bldFeature("가나아파트", "101동", bldLat, bldLon, 0.0003)
	overlap := bldFeature("가나상가", "", bldLat, bldLon+0.0002, 0.0001)
	near102 := bldFeature("가나아파트", "102동", bldLat, bldLon+0.0005, 0.0001) // 벽까지 약 35m
	unnamedWest := bldFeature("", "", bldLat, bldLon-0.0003, 0.0001)      // 벽까지 약 18m
	north := bldFeature("다라빌딩", "", bldLat+0.0006, bldLon, 0.0001)        // 벽까지 약 55m
	point := `{"type":"Feature","geometry":{"type":"Point","coordinates":[127,37.5]},"properties":{"buld_nm":"점"}}`
	cases := []struct {
		name, body, want string
	}{
		{"겹친 건물 벽이 더 가까워도 품은 건물이 먼저", vworldOK(overlap, in101), "가나아파트|101동 / 가나상가|"},
		{"가까운 순, 이름 없는 건물도 자리를 지킨다", vworldOK(north, near102, unnamedWest),
			"| / 가나아파트|102동 / 다라빌딩|"},
		{"이름 앞뒤 공백을 뗀다", vworldOK(bldFeature(" 가나빌딩 ", " 1동 ", bldLat, bldLon, 0.0002)), "가나빌딩|1동"},
		{"다각형이 아닌 건물은 뺀다", vworldOK(point, in101), "가나아파트|101동"},
		{"건물이 없으면 빈 목록", vworldOK(), ""},
		{"NOT_FOUND 면 빈 목록", `{"response":{"status":"NOT_FOUND"}}`, ""},
		{"Polygon 형식도 읽는다", vworldOK(strings.Replace(strings.Replace(in101,
			`"MultiPolygon","coordinates":[[`, `"Polygon","coordinates":[`, 1), `]]},"properties"`, `]},"properties"`, 1)),
			"가나아파트|101동"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := vworldServer(t, c.body, nil)
			if got := nearNames(s.buildingsNear(context.Background(), bldLat, bldLon)); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

// VWorld 는 POINT(경도 위도) 순서다. 바꿔 보내면 엉뚱한 곳의 건물이 온다.
func TestBuildingsNearRequest(t *testing.T) {
	var q url.Values
	s := vworldServer(t, vworldOK(), &q)
	s.buildingsNear(context.Background(), 37.5162897, 126.8625792)
	want := map[string]string{"data": "LT_C_SPBD", "key": "vk", "crs": "EPSG:4326", "geometry": "true",
		"geomFilter": "POINT(126.8625792 37.5162897)", "buffer": "30"}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s=%q, want %q", k, q.Get(k), v)
		}
	}
}

// 키가 없으면 VWorld 를 부르지 않고, 오류는 키·좌표 없이 원인만 로그에 남긴다.
func TestBuildingsNearFailures(t *testing.T) {
	called := false
	vw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		io.WriteString(w, `{"response":{"status":"ERROR","error":{"level":"2","code":"INVALID_KEY",`+
			`"text":"등록되지 않은 인증키입니다."}}}`)
	}))
	defer vw.Close()
	noKey := &Server{VWorldBase: vw.URL, HTTP: vw.Client(), Log: slog.New(slog.DiscardHandler)}
	if got := noKey.buildingsNear(context.Background(), bldLat, bldLon); got != nil || called {
		t.Errorf("키 없음: got %v called=%v", got, called)
	}

	var buf bytes.Buffer
	s := &Server{VWorldKey: "secretkey", VWorldBase: vw.URL, HTTP: vw.Client(),
		Log: slog.New(slog.NewTextHandler(&buf, nil))}
	if got := s.buildingsNear(context.Background(), bldLat, bldLon); got != nil {
		t.Errorf("ERROR 응답: got %v", got)
	}
	if !strings.Contains(buf.String(), "INVALID_KEY") {
		t.Errorf("오류 코드가 로그에 없다: %s", buf.String())
	}

	buf.Reset()
	down := &Server{VWorldKey: "secretkey", VWorldBase: "http://127.0.0.1:9", HTTP: &http.Client{},
		Log: slog.New(slog.NewTextHandler(&buf, nil))}
	if got := down.buildingsNear(context.Background(), 37.5662952, 126.9779692); got != nil {
		t.Errorf("연결 실패: got %v", got)
	}
	logged := buf.String()
	if !strings.Contains(logged, "vworld building") || strings.Contains(logged, "secretkey") ||
		strings.Contains(logged, "37.566") || strings.Contains(logged, "126.977") {
		t.Errorf("로그: %s", logged)
	}
}
