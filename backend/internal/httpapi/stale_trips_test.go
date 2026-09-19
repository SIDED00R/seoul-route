package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/speed"
)

// 오래 조용한 열린 trip 은 /end 와 같은 계산으로 닫히고, 방금 표본이 올라온 trip 은 건드리지 않는다.
func TestCloseStaleTrips(t *testing.T) {
	_, pool := testServer(t)
	ctx := context.Background()
	sub := "sub-stale-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM users WHERE google_sub = $1`, sub) })
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(google_sub) VALUES ($1) RETURNING id`, sub).Scan(&userID); err != nil {
		t.Fatalf("사용자 생성: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM speed_profiles WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM trips WHERE user_id = $1`, userID)
	})

	now := time.Now()
	// trip 하나를 만들고 시작 시각을 옮긴 뒤, 그 시각 언저리에 걷기 표본을 넣는다.
	newTrip := func(startedAgo time.Duration, samples int) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx,
			`INSERT INTO trips(user_id, started_at) VALUES ($1, $2) RETURNING id`,
			userID, now.Add(-startedAgo)).Scan(&id); err != nil {
			t.Fatalf("trip 생성: %v", err)
		}
		for i := 0; i < samples; i++ { // 5초 간격 1.3m/s 걷기
			at := now.Add(-startedAgo).Add(time.Duration(i) * 5 * time.Second)
			lat := 37.5 + float64(i)*6.5/111195
			if _, err := pool.Exec(ctx,
				`INSERT INTO traces(trip_id, ts, lat, lon, accuracy_m, mode, activity)
				 VALUES ($1, $2, $3, 127.0, 8, 'walk', 'walk')`, id, at, lat); err != nil {
				t.Fatalf("표본 삽입: %v", err)
			}
		}
		return id
	}
	stale := newTrip(speed.StaleAfter+time.Hour, 30)
	staleEmpty := newTrip(speed.StaleAfter+time.Hour, 0) // 표본 0개(위치 권한 거부·바로 취소)
	fresh := newTrip(speed.StaleAfter+time.Hour, 0)      // 시작은 오래됐지만 방금 표본이 올라온 긴 안내
	if _, err := pool.Exec(ctx,
		`INSERT INTO traces(trip_id, ts, lat, lon, accuracy_m, mode) VALUES ($1, $2, 37.5, 127.0, 8, 'walk')`,
		fresh, now.Add(-time.Minute)); err != nil {
		t.Fatalf("최근 표본: %v", err)
	}

	n, err := speed.CloseStaleTrips(ctx, pool, Priors, now)
	if err != nil {
		t.Fatalf("마감: %v", err)
	}
	if n != 2 {
		t.Errorf("닫은 trip %d개(기대 2)", n)
	}
	var walk *float64
	var ended bool
	if err := pool.QueryRow(ctx, `SELECT ended_at IS NOT NULL, walk_speed_mps FROM trips WHERE id = $1`,
		stale).Scan(&ended, &walk); err != nil {
		t.Fatalf("조회: %v", err)
	}
	if !ended || walk == nil || *walk < 1.0 || *walk > 1.6 {
		t.Errorf("표본 있는 trip: ended=%v walk=%v", ended, walk)
	}
	if err := pool.QueryRow(ctx, `SELECT ended_at IS NOT NULL, walk_speed_mps FROM trips WHERE id = $1`,
		staleEmpty).Scan(&ended, &walk); err != nil {
		t.Fatalf("조회: %v", err)
	}
	if !ended || walk != nil {
		t.Errorf("표본 없는 trip: ended=%v walk=%v", ended, walk)
	}
	if err := pool.QueryRow(ctx, `SELECT ended_at IS NOT NULL FROM trips WHERE id = $1`, fresh).Scan(&ended); err != nil {
		t.Fatalf("조회: %v", err)
	}
	if ended {
		t.Error("방금 표본이 올라온 trip 을 닫았다")
	}
	// 속도가 프로파일에 반영됐는지(표본 있는 trip 하나 분)
	var nTrips int
	if err := pool.QueryRow(ctx, `SELECT n_trips FROM speed_profiles WHERE user_id = $1 AND mode = 'walk'`,
		userID).Scan(&nTrips); err != nil {
		t.Fatalf("프로파일 조회: %v", err)
	}
	if nTrips != 1 {
		t.Errorf("프로파일 n_trips=%d(기대 1)", nTrips)
	}
	// 두 번 돌려도 이미 닫힌 trip 을 다시 세지 않는다.
	if again, err := speed.CloseStaleTrips(ctx, pool, Priors, now); err != nil || again != 0 {
		t.Errorf("두 번째 실행: n=%d err=%v", again, err)
	}
}
