-- 안내 궤적과 trip 별 속도. 원본 궤적은 30일 보관(speed.PurgeOldTraces) 후 지우고, 탈퇴 시 trip 과 함께 즉시 지운다.

CREATE TABLE traces (
    trip_id     UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    ts          TIMESTAMPTZ NOT NULL,           -- 앱의 위치 샘플 시각. (trip_id, ts) 중복 업로드는 무시한다
    lat         DOUBLE PRECISION NOT NULL,
    lon         DOUBLE PRECISION NOT NULL,
    accuracy_m  REAL NOT NULL,
    mode        TEXT NOT NULL CHECK (mode IN ('walk', 'bicycle', 'transit')), -- 샘플 시점의 안내 구간 수단
    PRIMARY KEY (trip_id, ts)
);
CREATE INDEX traces_ts_idx ON traces(ts);

-- trip 종료 시 산출한 이동 중 중앙값 속도(m/s). NULL 이면 그 수단의 표본이 부족했다.
ALTER TABLE trips
    ADD COLUMN walk_speed_mps DOUBLE PRECISION,
    ADD COLUMN bike_speed_mps DOUBLE PRECISION;

-- 수축 평균의 분자 Σ v_trip. speed_mps = (n0·μ0 + sum) / (n0 + n_trips).
ALTER TABLE speed_profiles ADD COLUMN sum_speed_mps DOUBLE PRECISION NOT NULL DEFAULT 0;
