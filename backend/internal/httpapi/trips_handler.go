package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/SIDED00R/seoul-route/backend/internal/route"
	"github.com/SIDED00R/seoul-route/backend/internal/speed"
)

// MaxTraceBatch: 한 업로드의 샘플 상한. 앱은 20개(5초 간격 100초)마다 올리므로 재전송 누적을 넉넉히 받는다. 앱 TraceUploader.maxPerRequest 와 같은 값.
// MaxClockSkew: 샘플 ts 가 서버 시각보다 이만큼 넘게 미래면 거부한다. 궤적 30일 삭제(speed.PurgeOldTraces)가 ts 기준이라
// 미래 ts 는 그만큼 오래 남기 때문. 5분은 휴대폰 시계 오차(보통 수 초)보다 넉넉한 값.
const (
	MaxTraceBatch = 1000
	MaxClockSkew  = 5 * time.Minute
)

// 수단별 사전값. 프로파일이 없거나 표본이 없는 사용자는 이 값으로 경로를 받는다(route 와 같은 상수).
var priors = map[string]float64{"walk": route.DefaultWalk, "bicycle": route.DefaultBike}

// handleStartTrip 은 안내 1회를 trip 으로 발급한다. 앱은 이 id 로만 궤적을 올린다.
func (s *Server) handleStartTrip(w http.ResponseWriter, r *http.Request) {
	var id string
	var startedAt time.Time
	err := s.DB.QueryRow(r.Context(), `INSERT INTO trips(user_id) VALUES ($1) RETURNING id, started_at`,
		userIDFrom(r.Context())).Scan(&id, &startedAt)
	if err != nil {
		s.Log.Error("start trip", "err", err)
		writeError(w, http.StatusInternalServerError, "trip 발급 실패")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"trip_id": id, "started_at": startedAt})
}

type traceSample struct {
	TS        time.Time `json:"ts"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	AccuracyM float64   `json:"accuracy_m"`
	Mode      string    `json:"mode"`
	Activity  string    `json:"activity"` // 폰 활동 인식 판정(speed.Activities), 없으면 빈 값
	// 앱이 활동 인식에서 받은 원시 판정과 신뢰도 등급(WALKING·IN_VEHICLE·HIGH 등). 진단용이라 값 목록을 검사하지 않고
	// 길이만 본다. 속도 학습은 Activity 만 쓴다.
	ActivityRaw  string `json:"activity_raw"`
	ActivityConf string `json:"activity_conf"`
}

// MaxActivityRawLen 은 activity_raw·activity_conf 의 최대 길이. 활동 인식 type·confidence 이름(IN_VEHICLE·HIGH)은
// 20자를 넘지 않는다.
const MaxActivityRawLen = 32

// nullIfEmpty 는 빈 문자열을 NULL 로 넣는다(traces 의 선택 열은 NULL 이 "없음").
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ownTrip 은 trip 이 요청 사용자 것인지와 종료 여부를 돌려준다. 남의 trip 은 404 로 숨긴다.
func (s *Server) ownTrip(w http.ResponseWriter, r *http.Request) (id string, ended bool, ok bool) {
	id = chi.URLParam(r, "id")
	err := s.DB.QueryRow(r.Context(), `SELECT ended_at IS NOT NULL FROM trips WHERE id = $1 AND user_id = $2`,
		id, userIDFrom(r.Context())).Scan(&ended)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "trip 없음")
	case err != nil: // id 가 UUID 형식이 아닐 때도 여기로 온다
		writeError(w, http.StatusNotFound, "trip 없음")
	default:
		ok = true
	}
	return
}

// handleUploadTraces 는 샘플 배치를 저장한다. (trip_id, ts) 가 같은 행은 무시하므로 앱이 실패분을 재전송해도 된다.
func (s *Server) handleUploadTraces(w http.ResponseWriter, r *http.Request) {
	id, ended, ok := s.ownTrip(w, r)
	if !ok {
		return
	}
	if ended {
		writeError(w, http.StatusConflict, "종료된 trip")
		return
	}
	var body struct {
		Samples []traceSample `json:"samples"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "요청 본문 오류")
		return
	}
	if len(body.Samples) == 0 || len(body.Samples) > MaxTraceBatch {
		writeError(w, http.StatusBadRequest, "samples 는 1~1000개")
		return
	}
	latest := time.Now().Add(MaxClockSkew)
	for _, p := range body.Samples {
		if p.TS.IsZero() || p.TS.After(latest) || p.Lat < route.MinLat || p.Lat > route.MaxLat ||
			p.Lon < route.MinLon || p.Lon > route.MaxLon || p.AccuracyM < 0 ||
			speed.MaxSpeed[p.Mode] == 0 && p.Mode != "transit" || p.Activity != "" && !speed.Activities[p.Activity] ||
			len(p.ActivityRaw) > MaxActivityRawLen || len(p.ActivityConf) > MaxActivityRawLen {
			writeError(w, http.StatusBadRequest,
				"샘플 오류(ts 미래·서울 밖 좌표·accuracy_m·mode walk/bicycle/transit·activity walk/bicycle/vehicle/still/unknown)")
			return
		}
	}
	batch := &pgx.Batch{}
	for _, p := range body.Samples {
		batch.Queue(`INSERT INTO traces(trip_id, ts, lat, lon, accuracy_m, mode, activity, activity_raw, activity_conf)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT DO NOTHING`,
			id, p.TS, p.Lat, p.Lon, p.AccuracyM, p.Mode, nullIfEmpty(p.Activity), nullIfEmpty(p.ActivityRaw),
			nullIfEmpty(p.ActivityConf))
	}
	if err := s.DB.SendBatch(r.Context(), batch).Close(); err != nil {
		s.Log.Error("upload traces", "err", err)
		writeError(w, http.StatusInternalServerError, "저장 실패")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": len(body.Samples)})
}

// handleEndTrip 은 trip 을 닫고 궤적으로 수단별 속도를 내 사용자 프로파일에 수축 반영한다. 두 번 부르면 409.
func (s *Server) handleEndTrip(w http.ResponseWriter, r *http.Request) {
	id, ended, ok := s.ownTrip(w, r)
	if !ok {
		return
	}
	if ended {
		writeError(w, http.StatusConflict, "이미 종료된 trip")
		return
	}
	ctx := r.Context()
	userID := userIDFrom(ctx)
	rows, err := s.DB.Query(ctx, `SELECT ts, lat, lon, accuracy_m, mode, COALESCE(activity, '') FROM traces
		WHERE trip_id = $1 ORDER BY ts`, id)
	if err != nil {
		s.Log.Error("end trip query", "err", err)
		writeError(w, http.StatusInternalServerError, "종료 실패")
		return
	}
	var samples []speed.Sample
	for rows.Next() {
		var p speed.Sample
		var acc float32
		if err := rows.Scan(&p.TS, &p.Lat, &p.Lon, &acc, &p.Mode, &p.Activity); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "종료 실패")
			return
		}
		p.AccuracyM = float64(acc)
		samples = append(samples, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "종료 실패")
		return
	}
	est := speed.TripSpeeds(samples)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "종료 실패")
		return
	}
	defer tx.Rollback(ctx)
	tripOut := map[string]any{}
	var walkV, bikeV *float64
	for _, mode := range []string{"walk", "bicycle"} {
		e, has := est[mode]
		entry := map[string]any{"pairs": e.Pairs, "mismatch": e.Mismatch, "used": has && e.OK}
		if has && e.Pairs > 0 { // 활동 불일치로만 채워진 수단은 속도가 없다(Mismatch 만 보고)
			entry["speed_mps"] = e.SpeedMps
		}
		tripOut[mode] = entry
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
			userID, mode, v, speed.Shrink(priors[mode], v, 1), speed.PriorTrips, priors[mode])
		if err != nil {
			s.Log.Error("profile upsert", "err", err)
			writeError(w, http.StatusInternalServerError, "종료 실패")
			return
		}
	}
	// ended_at IS NULL 조건이 종료 권한이다. 같은 trip 의 /end 가 겹치면(앱 타임아웃 뒤 재시도) 뒤진 쪽은 여기서 0행이라
	// 프로파일 갱신까지 롤백되고 409 를 받는다 — ownTrip 의 사전 검사는 트랜잭션 밖이라 이를 막지 못한다.
	tag, err := tx.Exec(ctx, `UPDATE trips SET ended_at = now(), walk_speed_mps = $2, bike_speed_mps = $3
		WHERE id = $1 AND ended_at IS NULL`, id, walkV, bikeV)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "종료 실패")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "이미 종료된 trip")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "종료 실패")
		return
	}
	profile, err := s.speedProfile(r)
	if err != nil {
		s.Log.Error("profile read", "err", err)
		writeError(w, http.StatusInternalServerError, "종료 실패")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"trip_id": id, "samples": len(samples), "trip": tripOut, "profile": profile})
}

// handleGetSpeed: GET /users/me/speed. 수단별 {speed_mps, n_trips, prior}. 표본이 없으면 speed_mps = prior.
func (s *Server) handleGetSpeed(w http.ResponseWriter, r *http.Request) {
	profile, err := s.speedProfile(r)
	if err != nil {
		s.Log.Error("profile read", "err", err)
		writeError(w, http.StatusInternalServerError, "조회 실패")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) speedProfile(r *http.Request) (map[string]any, error) {
	out := map[string]any{}
	for mode, prior := range priors {
		out[mode] = map[string]any{"speed_mps": prior, "n_trips": 0, "prior_mps": prior}
	}
	rows, err := s.DB.Query(r.Context(),
		`SELECT mode, n_trips, speed_mps FROM speed_profiles WHERE user_id = $1 AND speed_mps IS NOT NULL`,
		userIDFrom(r.Context()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var mode string
		var n int
		var v float64
		if err := rows.Scan(&mode, &n, &v); err != nil {
			return nil, err
		}
		out[mode] = map[string]any{"speed_mps": v, "n_trips": n, "prior_mps": priors[mode]}
	}
	return out, rows.Err()
}
