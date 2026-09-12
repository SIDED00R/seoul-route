// odcompare: 서울 대표 OD 를 우리 Planner(실시간 보정 포함)와 ODsay Lab API 로 한 실행에서 나란히 풀어 기록한다.
// 카카오·네이버는 대중교통 경로 API 가 없어 ODsay(개인 무료 30콜/일)를 참조계로 쓴다. 정답이 아니라 편향 방향을
// 보는 용도다. ODsay searchPubTransPathT 는 출발 시각 파라미터가 없어 시각 무관 대표값을 돌려주므로, 우리 쪽 출발
// 시각(지금 또는 -at)과 기준이 같지 않다. 결과는 docs/eval/<시각>.md 와 .json 으로 남긴다. ODSAY_API_KEY 가 없으면
// 안내하고 종료한다.
//
//	cd backend && go run ./cmd/odcompare            # OD 20쌍 = ODsay 20콜, 지금 출발(실시간 보정 포함)
//	cd backend && go run ./cmd/odcompare -n 5       # 앞 5쌍만
//	cd backend && go run ./cmd/odcompare -at 08:30  # 오늘 08:30 출발(시간표만, 실시간 없음). 새벽·심야 실행 시 사용
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/config"
	"github.com/SIDED00R/seoul-route/backend/internal/otp"
	"github.com/SIDED00R/seoul-route/backend/internal/realtime"
	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

// od 는 대표 구간. 이름은 앱과 같은 카카오 장소명 형식("…역")이라 역 앵커링이 걸린다. 좌표는 역 출입구 근처.
// 층화: 지하철 직행 / 환승 / 버스 우세 / 한강 횡단 / 외곽 / 대형역.
type od struct {
	Name         string
	OLat, OLon   float64
	DLat, DLon   float64
	OName, DName string
}

var ods = []od{
	{"서울역→강남역", 37.55406888733184, 126.97070335253385, 37.49808633653005, 127.02800140627488, "서울역", "강남역 2호선"},
	{"홍대입구→잠실", 37.5574, 126.9245, 37.5133, 127.1001, "홍대입구역 2호선", "잠실역 2호선"},
	{"여의도→수서", 37.5216, 126.9243, 37.4873, 127.1017, "여의도역 5호선", "수서역 3호선"},
	{"신촌→성수", 37.5551, 126.9368, 37.5445, 127.0559, "신촌역 2호선", "성수역 2호선"},
	{"종로3가→건대입구", 37.5713, 126.9915, 37.5403, 127.0700, "종로3가역 1호선", "건대입구역 2호선"},
	{"노원→왕십리", 37.6553, 127.0613, 37.5613, 127.0371, "노원역 4호선", "왕십리역 2호선"},
	{"사당→강남", 37.4766, 126.9816, 37.4979, 127.0276, "사당역 2호선", "강남역 2호선"},
	{"김포공항→여의도", 37.5623, 126.8010, 37.5216, 126.9243, "김포공항역 5호선", "여의도역 5호선"},
	{"용산→고속터미널", 37.5298, 126.9648, 37.5049, 127.0049, "용산역 1호선", "고속터미널역 3호선"},
	{"수유→종로3가", 37.6380, 127.0250, 37.5713, 126.9915, "수유역 4호선", "종로3가역 1호선"},
	{"구로디지털단지→신논현", 37.4852, 126.9015, 37.5045, 127.0250, "구로디지털단지역 2호선", "신논현역 9호선"},
	{"천호→시청", 37.5386, 127.1237, 37.5657, 126.9769, "천호역 5호선", "시청역 2호선"},
	{"상암→광화문", 37.5788, 126.8930, 37.5710, 126.9769, "디지털미디어시티역 6호선", "광화문역 5호선"},
	{"목동→압구정", 37.5262, 126.8646, 37.5271, 127.0284, "목동역 5호선", "압구정역 3호선"},
	{"석계→왕십리", 37.6146, 127.0657, 37.5613, 127.0371, "석계역 1호선", "왕십리역 2호선"},
	{"마포→이태원", 37.5395, 126.9457, 37.5345, 126.9944, "마포역 5호선", "이태원역 6호선"},
	{"잠실→강남", 37.5133, 127.1001, 37.4979, 127.0276, "잠실역 2호선", "강남역 2호선"},
	{"성북구청→동대문", 37.5934, 127.0173, 37.5714, 127.0094, "성신여대입구역 4호선", "동대문역 1호선"},
	{"염창→여의도", 37.5471, 126.8745, 37.5216, 126.9243, "염창역 9호선", "여의도역 5호선"},
	{"불광→신도림", 37.6103, 126.9296, 37.5088, 126.8912, "불광역 3호선", "신도림역 2호선"},
}

type row struct {
	Name      string   `json:"name"`
	OursMin   float64  `json:"ours_min"`      // 1순위 소요(출발부터 도착, 출발 대기 제외 — ODsay totalTime 과 같은 기준)
	OursWait  float64  `json:"ours_wait_min"` // 1순위 출발 대기
	OursSig   string   `json:"ours_sig"`
	OursTop3  []string `json:"ours_top3"`
	OdsayMin  float64  `json:"odsay_min"`
	OdsaySig  string   `json:"odsay_sig"`
	DeltaMin  float64  `json:"delta_min"` // ours − odsay
	Top3Match bool     `json:"top3_match"`
	Err       string   `json:"err,omitempty"`
}

func main() {
	n := flag.Int("n", len(ods), "대조할 OD 수")
	at := flag.String("at", "", "출발 시각 HH:MM(오늘). 비우면 지금 출발")
	flag.Parse()
	var depart *time.Time
	if *at != "" {
		hm, err := time.Parse("15:04", *at)
		if err != nil {
			fmt.Fprintln(os.Stderr, "-at 은 HH:MM 형식이다:", *at)
			os.Exit(2)
		}
		y, m, d := time.Now().Date()
		t := time.Date(y, m, d, hm.Hour(), hm.Minute(), 0, 0, time.Local)
		depart = &t
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cfg.ODsayKey == "" {
		fmt.Fprintln(os.Stderr, "ODSAY_API_KEY 가 없다. lab.odsay.com 에서 키를 받아 .env 에 넣는다(개인 무료 30콜/일).")
		os.Exit(2)
	}
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	otpClient := &otp.Client{URL: cfg.OTPURL, HTTP: &http.Client{Timeout: 90 * time.Second}}
	planner := &route.Planner{OTP: otpClient}
	if sts, err := otpClient.Stations(ctx); err == nil {
		planner.SetStations(sts)
	} else {
		log.Warn("stations", "err", err)
	}
	rt := &realtime.Corrector{Log: log}
	rtHTTP := &http.Client{Timeout: 5 * time.Second}
	if cfg.BusArrivalKey != "" {
		key := cfg.BusArrivalKey
		if d, err := url.QueryUnescape(key); err == nil {
			key = d
		}
		rt.Bus = &realtime.BusClient{Key: key, HTTP: rtHTTP}
	}
	if cfg.SubwayRealtimeKey != "" {
		rt.Subway = &realtime.SubwayClient{Key: cfg.SubwayRealtimeKey, HTTP: rtHTTP}
	}
	if rt.Bus != nil || rt.Subway != nil {
		planner.Realtime = rt
	}

	started := time.Now()
	var rows []row
	for i, o := range ods {
		if i >= *n {
			break
		}
		r := row{Name: o.Name}
		its, err := planner.Plan(ctx, route.PlanRequest{
			Origin:      route.Point{Lat: o.OLat, Lon: o.OLon, Name: o.OName},
			Destination: route.Point{Lat: o.DLat, Lon: o.DLon, Name: o.DName},
			Depart:      depart,
		})
		if err != nil {
			r.Err = "ours: " + err.Error()
		} else if len(its) > 0 {
			r.OursMin = its[0].Duration / 60
			r.OursWait = its[0].DepartIn / 60
			r.OursSig = sigOurs(its[0])
			for _, it := range its[:min(3, len(its))] {
				r.OursTop3 = append(r.OursTop3, sigOurs(it))
			}
		}
		om, osig, err := odsay(ctx, cfg.ODsayKey, o)
		if err != nil {
			r.Err += " odsay: " + err.Error()
		} else {
			r.OdsayMin, r.OdsaySig = om, osig
			r.DeltaMin = r.OursMin - om
			for _, s := range r.OursTop3 {
				if s == osig {
					r.Top3Match = true
				}
			}
		}
		fmt.Printf("%-16s ours %5.1f분(+대기 %4.1f) %-32s | odsay %5.1f분 %-32s | Δ %+5.1f %s %s\n",
			r.Name, r.OursMin, r.OursWait, r.OursSig, r.OdsayMin, r.OdsaySig, r.DeltaMin, mark(r.Top3Match), r.Err)
		rows = append(rows, r)
		time.Sleep(300 * time.Millisecond)
	}
	if err := write(rows, started, depart); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func mark(b bool) string {
	if b {
		return "top3✓"
	}
	return "top3✗"
}

// sigOurs 는 우리 itinerary 의 대중교통 수단열("4호선>2호선", "402").
func sigOurs(it otp.Itinerary) string {
	var parts []string
	for _, l := range it.Legs {
		if l.TransitLeg {
			parts = append(parts, normLine(l.Route))
		}
	}
	if len(parts) == 0 {
		return "도보/따릉이"
	}
	return strings.Join(parts, ">")
}

var lineRe = regexp.MustCompile(`([0-9]+호선|신분당선|공항철도|수인분당선|분당선|경의중앙선|경춘선|경강선|우이신설선|서해선|GTX-A|김포골드라인|신림선)`)

// normLine 은 "서울4호선"·"수도권 4호선"·"9호선(급행)" → "4호선"/"9호선". 버스 번호는 그대로.
func normLine(s string) string {
	if m := lineRe.FindString(s); m != "" {
		if m == "분당선" {
			return "수인분당선"
		}
		return m
	}
	return strings.TrimSpace(s)
}

// odsayHTTP 는 ODsay 전용 클라이언트. 응답이 멈추면 그 행만 오류로 남기고 다음 OD 로 넘어가도록 상한을 둔다.
var odsayHTTP = &http.Client{Timeout: 20 * time.Second}

// odsay 는 searchPubTransPathT 의 1순위 totalTime(분)과 수단열을 돌려준다.
func odsay(ctx context.Context, key string, o od) (float64, string, error) {
	q := url.Values{
		"SX": {fmt.Sprint(o.OLon)}, "SY": {fmt.Sprint(o.OLat)},
		"EX": {fmt.Sprint(o.DLon)}, "EY": {fmt.Sprint(o.DLat)},
		"apiKey": {key},
	}
	u := "https://api.odsay.com/v1/api/searchPubTransPathT?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, "", err
	}
	resp, err := odsayHTTP.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("요청 실패") // URL 에 키가 있어 원문은 남기지 않는다(타임아웃 오류도 URL 을 담는다)
	}
	defer resp.Body.Close()
	var out struct {
		Error  json.RawMessage `json:"error"`
		Result struct {
			Path []struct {
				Info    struct{ TotalTime float64 } `json:"info"`
				SubPath []struct {
					TrafficType int `json:"trafficType"` // 1 지하철 2 버스 3 도보
					Lane        []struct {
						Name  string `json:"name"`
						BusNo string `json:"busNo"`
					} `json:"lane"`
				} `json:"subPath"`
			} `json:"path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return 0, "", fmt.Errorf("응답 파싱 실패 (HTTP %d)", resp.StatusCode)
	}
	if len(out.Error) > 0 && string(out.Error) != "null" {
		return 0, "", fmt.Errorf("ODsay 오류 %s", trunc(string(out.Error), 120))
	}
	if len(out.Result.Path) == 0 {
		return 0, "", fmt.Errorf("경로 없음")
	}
	p := out.Result.Path[0]
	var parts []string
	for _, sp := range p.SubPath {
		if len(sp.Lane) == 0 {
			continue
		}
		switch sp.TrafficType {
		case 1:
			parts = append(parts, normLine(sp.Lane[0].Name))
		case 2:
			parts = append(parts, sp.Lane[0].BusNo)
		}
	}
	return p.Info.TotalTime, strings.Join(parts, ">"), nil
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// write 는 docs/eval/<시각>.md·.json 을 쓴다(레포 루트는 AGENTS.md 로 찾는다).
func write(rows []row, started time.Time, depart *time.Time) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "docs", "eval")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	base := filepath.Join(dir, started.Format("2006-01-02-150405"))
	js, _ := json.MarshalIndent(rows, "", "  ")
	if err := os.WriteFile(base+".json", js, 0o644); err != nil {
		return err
	}
	var deltas []float64
	match, ok := 0, 0
	for _, r := range rows {
		if r.Err != "" {
			continue
		}
		ok++
		deltas = append(deltas, r.DeltaMin)
		if r.Top3Match {
			match++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# ODsay 대조 %s\n\n", started.Format("2006-01-02 15:04"))
	when := "지금 출발(실시간 보정 포함)"
	if depart != nil {
		when = depart.Format("15:04") + " 출발(시간표만)"
	}
	fmt.Fprintf(&b, "OD %d쌍, 유효 %d. 우리 쪽은 %s. ODsay 는 출발 시각 파라미터가 없어 시각 무관 대표값. "+
		"소요 = 출발부터 도착(출발 대기 제외, ODsay totalTime 과 같은 기준). Δ = 우리 − ODsay(분).\n\n", len(rows), ok, when)
	if ok > 0 {
		sort.Float64s(deltas)
		abs := append([]float64(nil), deltas...)
		for i := range abs {
			if abs[i] < 0 {
				abs[i] = -abs[i]
			}
		}
		sort.Float64s(abs)
		fmt.Fprintf(&b, "- Δ 중앙값 %+.1f분 (사분위 %+.1f ~ %+.1f), |Δ| 중앙값 %.1f분\n", deltas[len(deltas)/2],
			deltas[len(deltas)/4], deltas[len(deltas)*3/4], abs[len(abs)/2])
		fmt.Fprintf(&b, "- ODsay 1순위 수단열이 우리 상위 3개 안: %d/%d (%.0f%%)\n\n", match, ok, 100*float64(match)/float64(ok))
	}
	b.WriteString("| OD | 우리 1순위 | 대기 | ODsay 1순위 | Δ | top3 |\n|---|---|---|---|---|---|\n")
	for _, r := range rows {
		if r.Err != "" {
			fmt.Fprintf(&b, "| %s | 오류: %s | | | | |\n", r.Name, r.Err)
			continue
		}
		fmt.Fprintf(&b, "| %s | %.1f분 %s | %.1f분 | %.1f분 %s | %+.1f | %s |\n", r.Name, r.OursMin, r.OursSig, r.OursWait,
			r.OdsayMin, r.OdsaySig, r.DeltaMin, mark(r.Top3Match))
	}
	if err := os.WriteFile(base+".md", []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Println("→", base+".md")
	return nil
}

func repoRoot() (string, error) {
	d, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "AGENTS.md")); err == nil {
			return d, nil
		}
		p := filepath.Dir(d)
		if p == d {
			return "", fmt.Errorf("레포 루트 없음")
		}
		d = p
	}
}
