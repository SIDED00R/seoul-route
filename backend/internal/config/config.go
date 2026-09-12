// Package config 는 환경변수와 레포 루트 .env 에서 설정을 읽는다. 환경변수가 .env 보다 우선한다.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port                string
	DatabaseURL         string
	OTPURL              string
	JWTSecret           string
	GoogleOAuthClientID string // 비어 있으면 /auth/google 이 503 을 돌려준다
	SeoulOpenAPIKey     string // 비어 있으면 따릉이 GBFS 어댑터를 띄우지 않는다
	PublicURL           string // OTP 가 GBFS 를 읽어갈 이 서버의 주소
	KakaoRESTKey        string // 비어 있으면 /places/search 가 503. 키는 서버에만 두고 앱에는 내려보내지 않는다
	VWorldKey           string // 비어 있으면 /tiles/* 가 503
	BusArrivalKey       string // 공공데이터포털 키(DATA_GO_KR_KEY). 비면 버스 첫 탑승 실시간 보정 없음
	SubwayRealtimeKey   string // 열린데이터광장 지하철 실시간 키. 비면 지하철 첫 탑승 실시간 보정 없음
	GTFSZip             string // 생성 GTFS zip(배차간격 표시용). 기본 <레포>/otp/data/seoul-gtfs.zip, 없으면 배차 표시 없음
	ODsayKey            string // ODsay Lab 키. cmd/odcompare(정확도 대조)에서만 쓴다. 서버는 안 쓴다
}

// Load 는 .env(있으면)를 읽고 환경변수로 덮어쓴 뒤 필수값을 검사한다.
func Load() (Config, error) {
	vals := map[string]string{}
	defaultGTFS := ""
	if root, err := repoRoot(); err == nil {
		readDotEnv(filepath.Join(root, ".env"), vals)
		defaultGTFS = filepath.Join(root, "otp", "data", "seoul-gtfs.zip")
	}
	get := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		if v := vals[k]; v != "" {
			return v
		}
		return def
	}
	c := Config{
		Port:                get("PORT", "8081"),
		DatabaseURL:         get("DATABASE_URL", ""),
		OTPURL:              strings.TrimRight(get("OTP_URL", "http://localhost:8080"), "/"),
		JWTSecret:           get("JWT_SECRET", ""),
		GoogleOAuthClientID: get("GOOGLE_OAUTH_CLIENT_ID", ""),
		SeoulOpenAPIKey:     get("SEOUL_OPENAPI_KEY", ""),
		KakaoRESTKey:        get("KAKAO_REST_API_KEY", ""),
		VWorldKey:           get("VWORLD_API_KEY", ""),
		BusArrivalKey:       get("DATA_GO_KR_KEY", ""),
		SubwayRealtimeKey:   get("SEOUL_SUBWAY_REALTIME_KEY", ""),
		GTFSZip:             get("GTFS_ZIP", defaultGTFS),
		ODsayKey:            get("ODSAY_API_KEY", ""),
	}
	c.PublicURL = strings.TrimRight(get("PUBLIC_URL", "http://localhost:"+c.Port), "/")
	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if len(c.JWTSecret) < 32 {
		missing = append(missing, "JWT_SECRET(32자 이상)")
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("설정 누락: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

func readDotEnv(path string, into map[string]string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			into[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
}

// repoRoot 는 실행 위치에서 위로 올라가며 AGENTS.md 가 있는 디렉터리를 찾는다(gtfsgen 과 같은 규칙).
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
			return "", errors.New("레포 루트 없음")
		}
		d = p
	}
}
