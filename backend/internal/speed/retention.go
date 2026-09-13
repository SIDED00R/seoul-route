package speed

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TraceRetention 30일: 원본 궤적은 학습 결과(trip 속도·프로파일)만 남기고 지운다(계획 v2 개인정보 항목).
const TraceRetention = 30 * 24 * time.Hour

// PurgeOldTraces 는 보관 기간이 지난 궤적 행을 지우고 지운 수를 돌려준다. main 이 1시간마다 부른다.
func PurgeOldTraces(ctx context.Context, pool *pgxpool.Pool, now time.Time) (int64, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM traces WHERE ts < $1`, now.Add(-TraceRetention))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
