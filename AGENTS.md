# seoul-route

서울시 한정 멀티모달 길찾기 서비스. 대중교통·따릉이·걷기를 한 경로 안에서 섞어 최저시간순으로 추천하고, 사용자별 실측 이동속도와 횡단보도 대기시간을 예상시간에 반영한다.

## 구조

```
otp/       OpenTripPlanner 2.10.0 설정·데이터 스크립트 (그래프 빌드는 로컬, 서빙은 VM)
backend/   Go API (chi + pgx/sqlc). OTP 오케스트레이션, 궤적 수집, 따릉이 GBFS 어댑터, 신호 잔여시간 조회
gtfs/      GTFS 생성기 (서울 버스 API + TAGO 지하철 시간표 → GTFS zip)
app/       Flutter 앱
deploy/    Compose·Caddy·GCE 배포
docs/      스파이크 보고서·설계 문서
```

## 실행

- OTP 그래프 빌드: `cd otp && java -Xmx6G -jar otp-shaded-2.10.0.jar --build --save .`
- OTP 서빙: `cd otp && java -Xmx4G -jar otp-shaded-2.10.0.jar --load .` → GraphQL `http://localhost:8080/otp/gtfs/v1`
- OSM 서울 추출: `python otp/extract_seoul.py` (입력 `otp/data/south-korea-latest.osm.pbf` → 출력 `otp/data/seoul.osm.pbf`)
- GBFS fixture 서버(스파이크용): `python -m http.server 8090 -d otp/fixtures/gbfs`
- 백엔드: `cd backend && go run ./cmd/api` (Phase 1부터)

## 검증

- Go: `cd backend && go test ./... && golangci-lint run`
- Flutter: `cd app && flutter analyze && flutter test`
- 기능 완료 판정은 테스트 통과가 아니라 실제 실행이다: OTP에 curl로 plan 요청을 보내 응답 본문을 읽고, 앱은 실기기에서 흐름을 태운다.

## 데이터·키

- 인증키는 `.env`(gitignore)에만 둔다. `.env.example`에 변수명이 있다. 채팅·로그·URL 출력에 키를 남기지 않는다.
- 국가교통DB GTFS 파일럿(2025-03 평일 1일)은 엔진 스파이크용이다. 운영 시간표는 `gtfs/` 생성기가 만든다.
- T-data 신호 잔여시간 API는 개발자 한도 하루 1,000건, 잔여시간 단위 1/10초, 좌표 없음.
- 열린데이터광장 `bikeList`는 1콜 최대 1,000행. 총건수는 `list_total_count`로 읽어 동적 페이징한다.

## 규칙

- Issue → 브랜치(`feat/…` `fix/…`) → PR(본문 `Closes #n`) → 리뷰 → Merge. main 직접 커밋 금지.
- 커밋 메시지에 AI 트레일러를 넣지 않는다.
- 파일은 단일 책임. 기능 추가는 새 파일로.
- 상수(속도 사전값·횡단보도 대기·필터 임계)는 출처·측정일·재보정 규칙을 주석으로 붙인다.
- 버전 고정: OTP 2.10.0, Go 1.27, PostgreSQL 17, Flutter stable(설치 시 확정).
