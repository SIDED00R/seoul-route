package speed

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TraceRetention 은 원본 위치 궤적의 보관 기간이다.
const TraceRetention = 30 * 24 * time.Hour

// PurgeOldTraces 는 보관 기간이 지난 궤적 행을 지우고 지운 수를 돌려준다. main 이 1시간마다 부른다.
func PurgeOldTraces(ctx context.Context, pool *pgxpool.Pool, now time.Time) (int64, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM traces WHERE ts < $1`, now.Add(-TraceRetention))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
