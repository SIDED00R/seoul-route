// gtfsgen: 서울 운영 GTFS 생성기.
//
//	gtfsgen fetch   서울 버스 API 를 호출해 gtfs/cache/ 에 저장한다(있으면 건너뜀). 일일 한도에 걸리면 중단, 다음 날 재실행.
//	gtfsgen build   캐시 + 국가교통DB 도시철도 파일럿(otp/data/202503_GTFS_DataSet) → gtfs/out/seoul-gtfs.zip
//
// 키는 .env 의 DATA_GO_KR_KEY. 키·URL 은 출력하지 않는다.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SIDED00R/seoul-route/gtfs/internal/build"
	"github.com/SIDED00R/seoul-route/gtfs/internal/kric"
	"github.com/SIDED00R/seoul-route/gtfs/internal/ktdb"
	"github.com/SIDED00R/seoul-route/gtfs/internal/osm"
	"github.com/SIDED00R/seoul-route/gtfs/internal/seoulbus"
	"github.com/SIDED00R/seoul-route/gtfs/internal/seoulmetro"
)

var seoulBBox = ktdb.BBox{MinLon: 126.70, MinLat: 37.38, MaxLon: 127.25, MaxLat: 37.75} // otp/extract_seoul.py 와 동일

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gtfsgen fetch|build")
		os.Exit(2)
	}
	root, err := repoRoot()
	if err != nil {
		fail(err)
	}
	cache := filepath.Join(root, "gtfs", "cache")
	switch os.Args[1] {
	case "fetch":
		key, err := envKey(filepath.Join(root, ".env"), "DATA_GO_KR_KEY")
		if err != nil {
			fail(err)
		}
		fail(fetch(seoulbus.New(key, cache)))
	case "build":
		fail(buildAll(root, cache))
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func fetch(c *seoulbus.Client) error {
	routes, err := c.AllRoutes()
	if err != nil {
		return err
	}
	done := 0
	for _, r := range routes {
		if c.Cached("stops_" + r.ID + ".json") {
			done++
		}
	}
	fmt.Printf("노선 %d개, 정류장 캐시 %d개 보유\n", len(routes), done)
	start := time.Now()
	for i, r := range routes {
		if _, err := c.StopsByRoute(r.ID); err != nil {
			if errors.Is(err, seoulbus.ErrQuota) {
				fmt.Printf("한도 도달: %d/%d 노선 캐시됨. 내일 다시 fetch 하면 이어서 받는다.\n", i, len(routes))
				return err
			}
			fmt.Printf("  %s(%s): %v\n", r.Name, r.ID, err)
			continue
		}
		if (i+1)%100 == 0 {
			fmt.Printf("  %d/%d (%s)\n", i+1, len(routes), time.Since(start).Round(time.Second))
		}
	}
	fmt.Println("fetch 완료")
	return nil
}

func buildAll(root, cache string) error {
	c := seoulbus.New("", cache) // 캐시 전용. 키 없이 API 를 부르면 실패하므로 캐시가 완전해야 한다.
	routes, err := c.AllRoutes()
	if err != nil {
		return fmt.Errorf("노선 목록 캐시 없음, 먼저 fetch: %w", err)
	}
	var buses []build.BusRoute
	missing := 0
	for _, r := range routes {
		if !c.Cached("stops_" + r.ID + ".json") {
			missing++
			continue
		}
		stops, err := c.StopsByRoute(r.ID)
		if err != nil {
			return err
		}
		buses = append(buses, build.BusRoute{Route: r, Stops: stops})
	}
	if missing > 0 {
		fmt.Printf("경고: 정류장 캐시 없는 노선 %d개 제외 (fetch 미완)\n", missing)
	}
	pilot := filepath.Join(root, "otp", "data", "202503_GTFS_DataSet")
	var subway *ktdb.Subway
	if st, err := os.Stat(pilot); err == nil && st.IsDir() {
		fmt.Println("도시철도: 국가교통DB 파일럿 로드 중 (stop_times 1.4GB 2회 스캔)")
		subway, err = ktdb.Load(pilot, seoulBBox)
		if err != nil {
			return err
		}
	} else {
		fmt.Println("경고: 파일럿 디렉터리 없음 — 버스만 생성")
	}
	outDir := filepath.Join(root, "gtfs", "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	out := filepath.Join(outDir, "seoul-gtfs.zip")
	// OSM 출입구(otp/extract_entrances.py 산출). 없으면 승강장 좌표 출입구로 폴백한다.
	entrancesCSV := filepath.Join(root, "otp", "data", "subway-entrances.csv")
	entrances, err := osm.LoadEntrances(entrancesCSV)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("경고: otp/data/subway-entrances.csv 없음 — 승강장 좌표 출입구로 생성 (python otp/extract_entrances.py)")
	} else if err != nil {
		return err
	}
	// 서울교통공사 열차운행시각표(otp/fetch_metro_timetable.py 산출). 없으면 파일럿 1~9호선 trip 그대로.
	var metro *seoulmetro.Timetable
	metroCSV := filepath.Join(root, "otp", "data", "seoul-metro-timetable.csv")
	if _, err := os.Stat(metroCSV); err == nil {
		fmt.Println("도시철도: 서울교통공사 열차운행시각표 로드 중")
		metro, err = seoulmetro.Load(metroCSV)
		if err != nil {
			return err
		}
	} else {
		fmt.Println("경고: otp/data/seoul-metro-timetable.csv 없음 — 1~9호선은 파일럿 시간표 (python otp/fetch_metro_timetable.py)")
	}
	// 레일포털 시각표(otp/fetch_kric_timetable.py 산출). 없으면 코레일·민자 노선은 파일럿 trip 그대로.
	var kricTT *kric.Timetable
	kricStations := filepath.Join(root, "otp", "data", "kric-stations.csv")
	kricTimetable := filepath.Join(root, "otp", "data", "kric-timetable.csv")
	if _, err := os.Stat(kricTimetable); err == nil {
		fmt.Println("도시철도: 레일포털 코레일·민자 노선 시각표 로드 중")
		kricTT, err = kric.Load(kricStations, kricTimetable)
		if err != nil {
			return err
		}
	} else {
		fmt.Println("경고: otp/data/kric-timetable.csv 없음 — 코레일·민자 노선은 파일럿 시간표 (python otp/fetch_kric_timetable.py)")
	}
	rep, err := build.Build(out, buses, subway, entrances, metro, kricTT)
	if err != nil {
		return err
	}
	printReport(rep, out)
	return nil
}

func printReport(rep *build.Report, out string) {
	skipped := 0
	for _, r := range rep.Routes {
		if r.Skipped != "" {
			skipped++
			fmt.Printf("  제외 %s(%s): %s\n", r.Name, r.RouteID, r.Skipped)
		}
	}
	fmt.Printf("버스 노선 %d (제외 %d), 정류장 %d | 도시철도 파일럿 trip %d(시각표로 대체 %d), 시각표 trip %d(빠진 정차 %d, "+
		"빠진 열차 %d, 이름으로 찾은 정차 %d, 시각 역행으로 뺀 열차 %d, 급행 통과역 %d, 시각 없는 정차 %d), 역 %d, "+
		"출입구 %d(OSM 출입구 붙은 역 %d, 승강장 좌표 폴백 %d, 출입구 통로 없는 승강장 %d), "+
		"통로 %d(부모역이 갈려 빠진 환승 %d, 거리 상한으로 뺀 쌍 %d) | 레일포털 노선 %d, trip %d(빠진 정차 %d, 빠진 열차 %d, "+
		"좌표로 붙인 역 %d, 시각 역행으로 뺀 열차 %d, 중복 행 %d) → %s\n",
		rep.NBusRoutes, skipped, rep.NBusStops, rep.NSubwayTrips, rep.NPilotTripsReplaced, rep.NMetroTrips,
		rep.NMetroSkippedStops, rep.NMetroSkippedTrips, rep.NMetroNameMatched, rep.NMetroNonMonotonic, rep.NMetroPassing,
		rep.NMetroNoTime, rep.NSubwayStops, rep.NEntrances,
		rep.NRealEntranceStations, rep.NFallbackEntranceStations, rep.NNoEntrancePlatforms, rep.NPathways,
		rep.NUnpairedTransfers, rep.NFarPairs, rep.NKricLines, rep.NKricTrips, rep.NKricSkippedStops, rep.NKricSkippedTrips,
		rep.NKricNearestMatched, rep.NKricNonMonotonic, rep.NKricDupRows, out)
}

func envKey(path, name string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf(".env 없음: %w", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, name+"=") {
			v := strings.TrimSpace(strings.TrimPrefix(line, name+"="))
			if strings.Contains(v, "%") { // 공공데이터포털 Encoding 키면 디코딩해 url.Values 인코딩에 맡긴다
				if d, err := url.QueryUnescape(v); err == nil {
					v = d
				}
			}
			if v == "" {
				return "", fmt.Errorf("%s 가 비어 있다", name)
			}
			return v, nil
		}
	}
	return "", fmt.Errorf("%s 가 .env 에 없다", name)
}

// repoRoot 는 실행 위치에서 위로 올라가며 공개 설정 예시(.env.example)가 있는 저장소 루트를 찾는다.
func repoRoot() (string, error) {
	d, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(d, ".env.example")); err == nil {
			return d, nil
		}
		p := filepath.Dir(d)
		if p == d {
			return "", errors.New("레포 루트(.env.example)를 찾지 못했다")
		}
		d = p
	}
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}
