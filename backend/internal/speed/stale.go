package speed

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StaleAfter: 마지막 표본(없으면 시작 시각)이 이만큼 조용하면 안내가 사라진 것으로 보고 trip 을 닫는다.
// 안내 중에는 5초마다 표본이 올라오므로(앱 TraceUploader.minGap) 이 간격이 비는 것은 안내가 없다는 뜻이다.
// 끊긴 동안 쌓아 두었다가 늦게 올리는 경우를 죽이지 않게 넉넉히 잡았다.
const StaleAfter = 6 * time.Hour

// CloseStaleTrips 는 오래 조용한 열린 trip 을 /end 와 같은 계산으로 닫는다(속도를 프로파일에 반영한 뒤 ended_at).
// 닫은 수를 돌려준다. 앱이 종료 요청을 보내지 못하고 죽으면 서버에는 정리할 경로가 없기 때문에 필요하다.
// main 이 궤적 정리와 같은 주기로 부른다.
func CloseStaleTrips(ctx context.Context, pool *pgxpool.Pool, priors map[string]float64, now time.Time) (int, error) {
	type trip struct{ id, userID string }
	rows, err := pool.Query(ctx, `SELECT t.id, t.user_id FROM trips t
		WHERE t.ended_at IS NULL
		  AND GREATEST(t.started_at, COALESCE((SELECT max(ts) FROM traces WHERE trip_id = t.id), t.started_at)) < $1
		ORDER BY t.started_at`, now.Add(-StaleAfter))
	if err != nil {
		return 0, err
	}
	var stale []trip
	for rows.Next() {
		var t trip
		if err := rows.Scan(&t.id, &t.userID); err != nil {
			rows.Close()
			return 0, err
		}
		stale = append(stale, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	closed := 0
	for _, t := range stale {
		ok, err := closeOne(ctx, pool, t.id, t.userID, priors)
		if err != nil {
			return closed, err
		}
		if ok {
			closed++
		}
	}
	return closed, nil
}

// closeOne 은 trip 하나를 한 트랜잭션에서 닫는다. 프로파일 갱신과 ended_at 이 함께 커밋돼야 표본이 두 번 반영되지 않는다.
func closeOne(ctx context.Context, pool *pgxpool.Pool, tripID, userID string, priors map[string]float64) (bool, error) {
	samples, err := LoadSamples(ctx, pool, tripID)
	if err != nil {
		return false, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	ok, err := Apply(ctx, tx, tripID, userID, TripSpeeds(samples), priors)
	if err != nil || !ok {
		return false, err
	}
	return true, tx.Commit(ctx)
}
