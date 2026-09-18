-- 앱이 활동 인식에서 받은 원시 판정(type)과 신뢰도 등급. activity 는 앱이 20초 유지 규칙으로 끈적하게 확정한 값이라,
-- 판정이 오지 않은 것과 직전 값이 유지된 것을 사후에 구분할 수 없다. 속도 학습에는 쓰지 않는다.
-- CHECK 제약은 두지 않는다 — 플러그인이 새 type 을 주면 배치 전체가 거절된다.
ALTER TABLE traces
    ADD COLUMN activity_raw TEXT,
    ADD COLUMN activity_conf TEXT;
