-- 샘플 시점에 폰 활동 인식이 판정한 실제 활동. NULL 은 앱이 활동 인식을 못 받은 샘플(권한 없음·구버전 앱).
-- 속도 학습은 활동이 walk/bicycle/vehicle 이면서 안내 구간 수단(mode)과 다른 샘플 쌍을 제외한다(speed.TripSpeeds).
ALTER TABLE traces
    ADD COLUMN activity TEXT CHECK (activity IN ('walk', 'bicycle', 'vehicle', 'still', 'unknown'));
