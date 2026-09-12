// devtoken: 개발용 JWT 발급. Google 로그인 없이 API 를 실행 검증할 때 쓴다(운영 배포 이미지에는 넣지 않는다).
//
//	go run ./cmd/devtoken <이름>   → google_sub "dev:<이름>" 사용자를 만들거나 찾아 토큰을 출력한다.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/SIDED00R/seoul-route/backend/internal/auth"
	"github.com/SIDED00R/seoul-route/backend/internal/config"
	"github.com/SIDED00R/seoul-route/backend/internal/db"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: devtoken <name>")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer pool.Close()
	var id string
	err = pool.QueryRow(ctx, `INSERT INTO users (google_sub) VALUES ($1)
		ON CONFLICT (google_sub) DO UPDATE SET google_sub = EXCLUDED.google_sub RETURNING id`, "dev:"+os.Args[1]).Scan(&id)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	tok, err := auth.NewJWT(cfg.JWTSecret).Issue(id)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(tok)
}
