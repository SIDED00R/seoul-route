# seoul-route

서울시 한정 멀티모달 길찾기. 대중교통·따릉이·걷기를 한 경로에서 섞어 최저시간순으로 추천하고, 사용자별 실측 이동속도와 횡단보도 대기시간을 예상시간에 반영한다.

- 라우팅 엔진: OpenTripPlanner 2.10.0 (OSM + GTFS + GBFS)
- 백엔드: Go 1.27 · PostgreSQL 17 + PostGIS
- 앱: Flutter

실행·검증·규칙은 [AGENTS.md](AGENTS.md).
