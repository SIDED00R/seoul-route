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
| via_yeouido_transit | 여의도 경유 | ✅ (GTFS 투입 후) 서울역→버스162→여의도→9호선→강남 등 5개. **`via` 는 대중교통 탐색에서 동작** |
| transit_rental_access / egress | 대중교통 + 따릉이 접근/이탈 | ✅ 응답은 정상이나 fixture 대여소 배치에서는 버스가 더 빨라 대여 leg 없음 → 아래 access_only 로 확인 |
| transit_rental_access_only / egress_only | 접근(이탈)을 따릉이로만 제한 | ❌ BadRequest — access/egress 에서도 `BICYCLE_RENTAL` 은 `WALK` 동반 필수 |
| transit_rental_*_slow_walker (걷기 0.6 m/s·reluctance 8) | 결합 leg 유도 | ✅ **"1호선 → 버스361 → 따릉이(신논현→강남)"** 등 대중교통+따릉이 결합 itinerary 생성. 미래 출발이라 빈 대여소 ST-5 를 썼다(제약 1 과 일치) |

GTFS 투입 후 케이스의 원본 출력은 `otp/spike/results-transit.txt`.

## GTFS 투입 후 그래프 (국가교통DB 파일럿, 서울 bbox 필터)

원본 전국 GTFS: stops 215,409 · routes 26,399 · trips 326,368 · stop_times 18,889,140 (1.4 GB). `otp/filter_gtfs_seoul.py` 로 bbox 안 정류장을 지나는 trip 만 남김(2분).

| 항목 | 값 |
|---|---|
| 필터 결과 | stops 38,640 · routes 3,060 · trips 150,112 · stop_times 8,662,836 · transfers 308, zip 85.6 MB |
| `route_type` | **파일럿 코드가 비표준**(설명서 4쪽: 0 시내버스, 1 도시철도, 3 시외버스, 5 공항버스 …). 필터에서 GTFS 표준(3 버스, 1 지하철, 2 철도, 4 페리, 1100 항공)으로 변환. 변환 전엔 시내버스가 TRAM 으로 잡힌다 |
| calendar | service B1 하나, 모든 요일 2017-01-01~2030-12-31 (평일 1일 시간표를 전 요일에 적용) |
| 빌드 | 1분 57초, **peak RSS 8,718 MB**, graph.obj 329 MB, 정점 504,910 · 간선 1,282,078 · 패턴 4,954 |
| 서빙 RSS (-Xmx6G, 기동 직후) | **5,138 MB** |
| DataImportIssue | IsolatedStop 10,994 (bbox 밖 정류장, 예상된 것) · StopNotLinkedForTransfers 11,562 · HopSpeedSlow 3,777 |

메모리 판정: 빌드 8.7 GB 는 8 GB VM 불가 → **그래프는 로컬(32 GB)에서 빌드해 artifact 로 배포**한다(계획대로). 서빙 5.1 GB 도 e2-standard-2 에서 PostgreSQL 과 공존이 빠듯하다 → 운영 GTFS 생성기는 **서울 노선만**(시외·전국 통과 노선 제외) 만들어 그래프를 줄이거나 e2-standard-4 로 간다. Phase 1a 에서 재측정.

### 실측으로 확정된 OTP 2.10 제약 (설계 반영)

1. **실시간 잔여대수는 "지금 출발" 요청에서만 라우팅에 쓰인다.** GTFS GraphQL API 가 `useAvailabilityInformation = isTripPlannedForNow` 로 덮어쓴다(`RouteRequestMapper.setRentalAvailabilityPreferences`). router-config 의 `useAvailabilityInformation: true` 는 이 API 경로에서 무의미. 미래 출발 계획은 잔여대수를 못 보는 게 맞으므로 제품 동작으로 수용한다.
2. **`via` 는 대중교통 탐색 전용.** 스키마 주석: "정류장에서 걸어가 방문하고 다른 정류장으로 돌아와 대중교통에 합류". `directOnly` 탐색에서는 경로 없음. → 도보·자전거만으로 경유지를 도는 구간은 백엔드가 구간별 호출을 이어 붙인다.
3. `modes` 를 생략하고 `via` 를 주면 서버 NPE(`modesArgs is null`). 항상 `modes` 를 명시한다.
4. `direct: [BICYCLE_RENTAL]` 단독은 BadRequest — 같은 요청에 `WALK` 를 함께 넣어야 한다.
5. OTP 2.10.0 은 Java 25 필수.

## 외부 API 표본 실측 (2026-09-12, `otp/spike/probe_apis.py`, 원본 `otp/spike/results-apis.txt`)

| API | 결과 | GTFS/GBFS 관점 |
|---|---|---|
| 따릉이 `bikeList` (열린데이터광장) | ✅ 대여소 **2,734곳**, 1,000행/콜 → **3콜**. 필드: stationId·stationName·stationLatitude/Longitude·rackTotCnt·parkingBikeTotCnt·shared | station_information + station_status 를 이 한 API 로 만들 수 있다(마스터 API 불필요). **`list_total_count` 는 요청 범위 건수**를 돌려주므로 짧은 페이지까지 넘겨 센다 |
| 서울 버스 `busRouteInfo` (공공데이터포털) | ✅ `getBusRouteList`(노선 검색), `getRouteInfo`(첫차·막차·배차간격 `term`), `getStaionByRoute`(정류장 123개/271번: seq·gpsX/Y·fullSectDist·sectSpd·direction·transYn·**정류장별 beginTm/lastTm**), `getRoutePath`(좌표 1,633점) | routes/stops/stop_times(frequencies)/shapes 전부 확보 가능. 정류장별 첫막차가 있어 구간 소요 추정에 쓸 수 있다 |
| TAGO 지하철 `SubwayInfo` (공공데이터포털) | ✅ 경로는 `/1613000/SubwayInfo/Get…`(2022-09 개편, 구 `SubwayInfoService/get…` 는 오류 12). 역 검색 20건, 역별 시간표(서울역 공항철도 평일 상행) 209건: depTime·arrTime·endSubwayStationId·subwayRouteId·dailyTypeCode(01/02/03)·upDownTypeCode(U/D) | stop_times 직접 생성 가능. **역 좌표 없음** → 좌표는 열린데이터광장 지하철역 좌표 또는 KTDB 파일럿 stops 에서 결합 |
| T-data 신호 잔여시간 `v2xSignalPhaseTimingInformation/1.0` | ⚠️ 키 승인됨. 2026-09-12 14:27~14:28 호출 3회(numOfRows 1000/100/10) 모두 **서버 500** (`CannotGetJdbcConnectionException`, T-data 측 DB 장애). 게이트웨이는 통과했으므로 키는 유효 | 재시도 후 교차로 수·단위·신선도 실측. 엔드포인트는 `apig/apiman-gateway/tapi/…?apikey=` 형식 |

## 게이트 판정

- (a) OTP 혼합 경로 실증: **통과**. 도로망+GBFS(따릉이 대여 경로·속도 파라미터), 대중교통 결합(버스·지하철 레이블 정상), 대중교통+따릉이 결합 itinerary, 대중교통 탐색에서의 `via` 경유지까지 실측. 제약은 위 5건.
- (b) T-data 매핑 가능 여부: **미판정**. 키는 승인·유효하나 2026-09-12 14:27~14:54 4회 모두 T-data 서버 500(DB 장애). 복구 후 재실측.
- (c) GTFS 생성기 착수 가능 여부: **가능**. 버스는 배차간격 기반 frequencies, 지하철은 역별 시간표 기반 정확 시각. 남은 확인 = 지하철 역 좌표 소스, 버스 정류장 간 소요시간 추정 방식(정류장별 첫막차 차이 또는 구간거리÷속도).
