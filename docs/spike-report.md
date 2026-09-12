# Phase 0 스파이크 보고서

작성일: 2026-09-12. 이슈 #1.

## 환경

| 항목 | 결과 |
|---|---|
| Java | Temurin **25.0.4.1** (포터블, `C:\Users\SAMSUNG\tools\jdk-25.0.4.1+1`, JAVA_HOME·PATH 사용자 변수 등록). Java 21 로는 OTP 2.10.0 이 `UnsupportedClassVersionError`(class 69) 로 기동 실패 — 실측 |
| Go | 1.27.1 (포터블, `C:\Users\SAMSUNG\tools\go`) |
| Docker Desktop | (기록 예정) |
| OTP | 2.10.0 (`otp/otp-shaded-2.10.0.jar`, 2026-09-09 릴리스) |
| OSM | Geofabrik south-korea-latest (2026-09-12 다운로드, 287MB) → 서울 bbox 126.70~127.25E, 37.38~37.75N |

## OSM 추출

`otp/extract_seoul.py` (pyosmium 4.3.1, 3패스 complete_ways). 소요 3분 8초 (12:59:59 → 13:03:07).

| 항목 | 값 |
|---|---|
| bbox 안 노드 | 4,111,416 |
| 채택 way | 639,420 (way 완결로 노드 4,160,297) |
| 채택 relation | 16,616 |
| 출력 `data/seoul.osm.pbf` | 40.8 MB (입력 한국 전체 287 MB) |

교훈: pyosmium `type_str()` 은 `"n"/"w"/"r"` 한 글자를 돌려준다. `"node"` 와 비교하면 0건이 써진다(실측 1회 재실행).

## 그래프 빌드 (도로망만, GTFS 없음)

`java -Xmx8G -jar otp-shaded-2.10.0.jar --build --save .` (JDK 25, 2026-09-12 13:04)

| 항목 | 값 |
|---|---|
| 빌드 소요 | 27초 (프로세스 전체 30초) |
| 빌드 peak RSS | 2,516 MB |
| graph.obj | 69.3 MB |
| 정점 / 간선 | 413,370 / 1,119,632 |
| 서빙 RSS (`--load`, -Xmx4G, 기동 직후) | 909 MB |
| DataImportIssue | GraphIslandWALK 2,103 · VertexWithoutEdges 4,968 · InvalidOsmGeometry 13 (경고 수준) |

GTFS 투입 후 재측정 필요. 도로망만으로는 8GB VM 여유가 충분하다.

## planConnection 실증 (도로망 + GBFS fixture 5곳)

`otp/spike/run_spike.py`. 원본 출력 `otp/spike/results-street-only.txt`.

| 케이스 | 기대 | 결과 |
|---|---|---|
| walk_only (서울역→용산역 3.88km) | 도보 leg 1개 | ✅ 56.6분 (walk.speed 1.2) |
| walk_speed_1.0 / 1.6 | 속도에 반비례 | ✅ 68.0분 / 42.5분 — 요청별 `preferences.street.walk.speed` 반영 |
| rental_direct (서울역→여의도) | WALK → BICYCLE(rent) → WALK | ✅ 44.6분 = 도보 5.8 + 자전거 38.8(7.19km, ST-1→ST-2) |
| rental_direct_bike_speed_2.5 | 자전거 leg 증가 | ✅ 자전거 38.8→53.7분 — `preferences.street.bicycle.speed` 반영 |
| rental_from_empty_station (ST-5 0대) | 빈 대여소 미사용 | ⚠️ **출발시각을 미래로 주면 0대 대여소에서 대여함.** `SPIKE_DEPART=now` 로 지금 출발이면 ✅ 회피(도보 909m) |
| via_yeouido_direct_only | 여의도 경유 | ❌ `NO_DIRECT_MODE_CONNECTION` — via 는 도보/자전거 전용(direct) 탐색에서 동작하지 않음 |
| via_yeouido_transit | 여의도 경유 | ⏳ GTFS 없어 `OUTSIDE_SERVICE_PERIOD`. GTFS 투입 후 재검증 |
| transit_rental_access / egress | 대중교통 + 따릉이 접근/이탈 | ⏳ GTFS 투입 후 |

### 실측으로 확정된 OTP 2.10 제약 (설계 반영)

1. **실시간 잔여대수는 "지금 출발" 요청에서만 라우팅에 쓰인다.** GTFS GraphQL API 가 `useAvailabilityInformation = isTripPlannedForNow` 로 덮어쓴다(`RouteRequestMapper.setRentalAvailabilityPreferences`). router-config 의 `useAvailabilityInformation: true` 는 이 API 경로에서 무의미. 미래 출발 계획은 잔여대수를 못 보는 게 맞으므로 제품 동작으로 수용한다.
2. **`via` 는 대중교통 탐색 전용.** 스키마 주석: "정류장에서 걸어가 방문하고 다른 정류장으로 돌아와 대중교통에 합류". `directOnly` 탐색에서는 경로 없음. → 도보·자전거만으로 경유지를 도는 구간은 백엔드가 구간별 호출을 이어 붙인다.
3. `modes` 를 생략하고 `via` 를 주면 서버 NPE(`modesArgs is null`). 항상 `modes` 를 명시한다.
4. `direct: [BICYCLE_RENTAL]` 단독은 BadRequest — 같은 요청에 `WALK` 를 함께 넣어야 한다.
5. OTP 2.10.0 은 Java 25 필수.

## 외부 API 표본 (인증키 도착 후)

- bikeList: 총건수, 페이지 수, 응답 필드
- T-data 10120: 응답 교차로 수, 단위, 좌표 매핑 자료 유무
- 서울 버스 API: 노선·정류장·배차 응답 형식

## 게이트 판정

- (a) OTP 혼합 경로 실증:
- (b) T-data 매핑 가능 여부:
- (c) GTFS 생성기 착수 가능 여부:
