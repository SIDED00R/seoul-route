-- 최근 경로: 사용자가 찾았던 출발지→도착지(경유지·구간 수단 포함). 홈의 "최근 경로" 탭이 읽어 다시 검색한다.
-- 같은 요청은 한 줄로 묶고 마지막 검색 시각만 갱신한다(dedup_key). 요청 본문은 그대로 두어 경유지·수단 고정까지
-- 그대로 재현한다. 탈퇴하면 사용자 행과 함께 지운다.
CREATE TABLE recent_routes (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    dedup_key   TEXT NOT NULL,                  -- 좌표를 소수 5자리(약 1m)로 줄인 출발·경유·도착과 구간 수단
    request     JSONB NOT NULL,                 -- 그 검색의 요청(origin/destination/via/segment_modes)
    searched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, dedup_key)
);
CREATE INDEX recent_routes_user_time_idx ON recent_routes(user_id, searched_at DESC);
