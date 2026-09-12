-- 사용자·이동·속도 프로파일 초기 스키마. PostGIS 는 Phase 4(횡단보도)에서 별도 마이그레이션으로 켠다.

CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    google_sub  TEXT NOT NULL UNIQUE,          -- Google ID 토큰의 sub. 이메일·이름은 저장하지 않는다
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ                     -- 탈퇴 시각. 원본 궤적은 즉시 삭제, 행은 감사용으로 남긴다
);

-- 안내 1회 = trip. 앱은 서버가 발급한 trip id 로만 궤적을 올린다.
CREATE TABLE trips (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id),
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at    TIMESTAMPTZ
);
CREATE INDEX trips_user_id_idx ON trips(user_id);

-- 사용자별 이동 모드(walk/bicycle) 속도 프로파일. 값은 Phase 3 에서 채운다.
CREATE TABLE speed_profiles (
    user_id     UUID NOT NULL REFERENCES users(id),
    mode        TEXT NOT NULL CHECK (mode IN ('walk', 'bicycle')),
    n_trips     INTEGER NOT NULL DEFAULT 0,
    speed_mps   DOUBLE PRECISION,               -- 수축 평균. NULL 이면 사전값 사용
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, mode)
);
