-- 사용자가 직접 고른 빠른 목적지. 집·회사는 계정마다 하나, 전체는 API 에서 20개로 제한한다.
CREATE TABLE favorite_places (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('home', 'work', 'custom')),
    label       TEXT NOT NULL CHECK (char_length(label) BETWEEN 1 AND 20),
    place_name  TEXT NOT NULL,
    address     TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL DEFAULT '',
    lat         DOUBLE PRECISION NOT NULL,
    lon         DOUBLE PRECISION NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX favorite_places_user_created_idx ON favorite_places(user_id, created_at);
CREATE UNIQUE INDEX favorite_places_home_work_idx ON favorite_places(user_id, kind)
    WHERE kind IN ('home', 'work');
