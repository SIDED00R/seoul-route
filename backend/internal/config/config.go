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
}

// Load 는 .env(있으면)를 읽고 환경변수로 덮어쓴 뒤 필수값을 검사한다.
func Load() (Config, error) {
	vals := map[string]string{}
	if root, err := repoRoot(); err == nil {
		readDotEnv(filepath.Join(root, ".env"), vals)
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
