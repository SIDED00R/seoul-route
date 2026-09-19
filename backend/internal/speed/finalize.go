package speed

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DB 는 이 파일이 쓰는 만큼의 데이터베이스. pgxpool.Pool 과 pgx.Tx 가 모두 만족한다.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// LoadSamples 는 trip 의 궤적을 시각순으로 읽는다.
func LoadSamples(ctx context.Context, db DB, tripID string) ([]Sample, error) {
	rows, err := db.Query(ctx, `SELECT ts, lat, lon, accuracy_m, mode, COALESCE(activity, '') FROM traces
		WHERE trip_id = $1 ORDER BY ts`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sample
	for rows.Next() {
		var p Sample
		var acc float32
		if err := rows.Scan(&p.TS, &p.Lat, &p.Lon, &acc, &p.Mode, &p.Activity); err != nil {
			return nil, err
		}
		p.AccuracyM = float64(acc)
		out = append(out, p)
	}
	return out, rows.Err()
}

// Apply 는 trip 하나의 수단별 속도를 사용자 프로파일에 수축 반영하고 그 trip 을 닫는다. 닫았으면 true.
// `ended_at IS NULL` 조건이 종료 권한이다 — 같은 trip 을 둘이 동시에 닫으려 하면 뒤진 쪽이 false 를 받고,
// 호출자가 트랜잭션을 되돌려 프로파일 갱신도 함께 취소한다. priors 는 수단별 사전값(walk·bicycle).
func Apply(ctx context.Context, tx DB, tripID, userID string, est map[string]Estimate,
	priors map[string]float64) (bool, error) {
	var walkV, bikeV *float64
	for _, mode := range []string{"walk", "bicycle"} {
		e, has := est[mode]
		if !has || !e.OK {
			continue
		}
		v := e.SpeedMps
		if mode == "walk" {
			walkV = &v
		} else {
			bikeV = &v
		}
		_, err := tx.Exec(ctx, `INSERT INTO speed_profiles(user_id, mode, n_trips, sum_speed_mps, speed_mps, updated_at)
			VALUES ($1, $2, 1, $3, $4, now())
			ON CONFLICT (user_id, mode) DO UPDATE SET n_trips = speed_profiles.n_trips + 1,
				sum_speed_mps = speed_profiles.sum_speed_mps + EXCLUDED.sum_speed_mps,
				speed_mps = ($5::float8 * $6::float8 + speed_profiles.sum_speed_mps + EXCLUDED.sum_speed_mps)
					/ ($5::float8 + speed_profiles.n_trips + 1),
				updated_at = now()`,
			userID, mode, v, Shrink(priors[mode], v, 1), PriorTrips, priors[mode])
		if err != nil {
			return false, err
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE trips SET ended_at = now(), walk_speed_mps = $2, bike_speed_mps = $3
		WHERE id = $1 AND ended_at IS NULL`, tripID, walkV, bikeV)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
