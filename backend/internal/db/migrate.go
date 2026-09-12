// Package db 는 pgx 연결과 SQL 마이그레이션 러너다.
// migrations/NNNN_name.sql 을 파일명 순서로 적용하고 schema_migrations 에 기록한다. 되돌리기(down)는 없다.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Connect 는 풀을 만들고 1회 ping 한다.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

// Migrate 는 미적용 마이그레이션을 각각 트랜잭션 안에서 적용하고 적용한 파일명을 돌려준다.
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	applied := map[string]bool{}
	rows, err := pool.Query(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		applied[n] = true
	}
	rows.Close()
	// Next 는 오류로 끊겨도 false 를 돌려주므로 Err 를 봐야 한다. 안 보면 적용 목록이 비어 기존 SQL 을 재실행한다.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	names, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var done []string
	for _, name := range names {
		if applied[name] {
			continue
		}
		sqlText, err := migrationFS.ReadFile(name)
		if err != nil {
			return nil, err
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, string(sqlText)); err != nil {
			tx.Rollback(ctx)
			return nil, fmt.Errorf("migrate %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES ($1)`, name); err != nil {
			tx.Rollback(ctx)
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		done = append(done, name)
	}
	return done, nil
}
