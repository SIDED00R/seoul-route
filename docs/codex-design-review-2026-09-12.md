총평: “알고리즘보다 데이터·모델링이 문제”라는 진단은 정확합니다. 다만 현재 변경은 서울역→강남역 한 사례의 시간을 카카오에 맞추는 방향에 가깝고, 실제 운행·환승 가능성을 안정적으로 재현하는 구조는 아직 아닙니다. 현 상태에서 OTP 알고리즘을 바꿔도 카카오 수준에는 도달하기 어렵습니다.

1. `otp/router-config.json:19` — 실시간 대중교통 데이터가 라우팅에 연결되지 않았습니다.

- 근거: updater는 따릉이 `vehicle-rental` 하나뿐입니다. 지금 출발 경로도 버스·지하철 지연, 결행, 실제 도착예정 시간을 사용하지 않습니다.
- 결과: 정적 GTFS가 아무리 정교해도 사고·정체·배차 불규칙이 큰 버스에서 카카오 수준 ETA를 낼 수 없습니다.
- 수정 제안: 서울 버스 도착정보를 GTFS-RT `TripUpdates`로 변환하는 어댑터를 최우선으로 두십시오. 차량 위치는 보조이고, 라우팅에는 정류장별 예측 도착시각이 핵심입니다. 서울 API는 첫·두 번째 차량의 예측 도착시간과 혼잡도까지 제공합니다. OTP도 실시간 TripUpdates를 즉시 라우팅에 적용할 수 있습니다. [서울 버스도착정보](https://www.data.go.kr/dataset/15000314/openapi.do?lang=en), [OTP GTFS-RT 설정](https://docs.opentripplanner.org/en/dev-2.x/GTFS-RT-Config/)
- 심각도: 🔴 반드시 수정

2. [gtfs/internal/build/build.go:197](/C:/Users/SAMSUNG/Desktop/seoul-route/gtfs/internal/build/build.go:197) — 평균 배차간격만으로 `exact_times=1`을 만드는 것은 존재하지 않는 고정 시간표를 발명합니다.

- 근거: `firstBus + n × term`을 하루 종일 정확한 출발시각으로 취급합니다. 그러나 원천 API의 `term`은 고정 시각표가 아니라 대표 배차간격입니다.
- `exact_times=1`은 “실제로 정확히 그 간격으로 운행되는 일정 기반 서비스”에만 쓰는 값입니다. `0`은 비고정 배차 서비스입니다. [GTFS 공식 규격](https://gtfs.org/documentation/schedule/reference/)
- 영향: 특정 출발시각에서는 대기가 우연히 0분이 되고, 몇 분 차이로 노선 순위가 크게 뒤집힙니다. 서울역→강남 시간이 카카오와 가까워진 것은 정확도 향상보다 위상 맞춤일 수 있습니다.
- 수정 제안: 실시간 도착정보가 있으면 그 값을 사용하고, 없으면 시간대별 관측 headway의 기대대기·상위 분위수를 별도 비용으로 모델링하십시오. 실제 출발 로그가 확보됐을 때만 `exact_times=1` 또는 절대 `stop_times`를 생성해야 합니다.
- 심각도: 🔴 반드시 수정

3. [gtfs/internal/build/build.go:200](/C:/Users/SAMSUNG/Desktop/seoul-route/gtfs/internal/build/build.go:200) — 변동하는 `sectSpd` 한 번의 스냅샷과 10분 표본 정차시간을 장기 정적 시간표로 고정합니다.

- 근거: 문서 스스로 `sectSpd`가 70초 사이에도 바뀐다고 기록하면서, 토요일 밤 10분 표본에서 얻은 간선 38초·지선 30초를 모든 정류장과 시간대에 더합니다.
- 영향: 첨두/비첨두, 평일/주말, 중앙차로/가로변, 승하차 수요 차이를 표현하지 못합니다. 오차를 전부 정차시간으로 귀속했기 때문에 신호대기·혼잡·위치 API 양자화까지 중복 가산할 수 있습니다.
- 수정 제안: `노선×방향×구간×요일유형×15~30분 시간대` 단위의 중앙값과 P85 주행시간을 장기간 수집하십시오. 정차시간은 승하차량·정류장 유형과 분리 추정하고, 학습 기간과 다른 날짜를 holdout으로 검증해야 합니다.
- 심각도: 🔴 반드시 수정

4. [docs/gtfs-generator.md:43](/C:/Users/SAMSUNG/Desktop/seoul-route/docs/gtfs-generator.md:43) — 2025년 평일 도시철도 파일럿을 2026년 전 요일에 적용합니다.

- 근거: 버스와 도시철도 모두 `service_id=ALL`이고 월~일이 전부 활성화됩니다. 주말·공휴일 감축 운행과 시간표 개정이 사라집니다.
- 영향: 존재하지 않는 열차를 추천하거나 실제 열차를 놓치며, 막차 경로는 특히 크게 틀립니다.
- 수정 제안: 최신 운영사별 시간표를 평일·토요일·휴일 서비스로 분리하고 `calendar_dates.txt`에 임시 변경과 공휴일을 넣으십시오. 서울교통공사는 이미 세 요일 유형의 역별 시간표를 제공합니다. [서울교통공사 시간표 API](https://www.data.go.kr/data/15056777/openapi.do)
- 심각도: 🔴 반드시 수정

5. [gtfs/internal/seoulbus/types.go:32](/C:/Users/SAMSUNG/Desktop/seoul-route/gtfs/internal/seoulbus/types.go:32) — 회차지 정보가 수집되지만 GTFS 생성에서 무시됩니다.

- 근거: `TransYn`이 존재하는데 전체 `b.Stops`를 하나의 trip으로 만들고, 모든 구간에 동일한 `direction_id=0`과 종점 headsign을 붙입니다.
- 영향: 회차지를 지나 반대 방향까지 하차 없이 계속 타는 경로, 회차 대기 0초 경로, 반대 방향 headsign 오류가 생길 수 있습니다.
- 수정 제안: `TransYn=Y`에서 상·하행 trip을 분리하십시오. 실제 동일 차량 연속운행이 확인되면 `block_id`와 현실적인 회차시간으로 연결해야 합니다.
- 심각도: 🔴 반드시 수정

6. [backend/internal/route/anchor.go:10](/C:/Users/SAMSUNG/Desktop/seoul-route/backend/internal/route/anchor.go:10) — 역 앵커링은 스냅 문제를 해결한 것이 아니라 역 접근시간을 우회합니다.

- 근거: 장소 좌표를 최대 1km 떨어진 이름 기반 부모역으로 바꾸고, 부모역 좌표도 자식 stop의 평균으로 합성합니다. 실제 출입구, 개찰구, 층, 승강장 간 연결은 없습니다.
- 영향: 서울역·고속터미널·왕십리 같은 대형역에서 잘못된 출입구와 불가능한 단거리 환승을 만들 수 있습니다. 54.3→39.8분 감소에는 제거된 우회뿐 아니라 실제 역사 진입시간까지 빠졌을 가능성이 있습니다.
- 수정 제안: 입력 좌표를 유지하고 출입구 후보로 스냅한 뒤 `entrance → pathway → platform`을 모델링하십시오. 당장 어렵다면 `역×노선쌍×방향` 환승시간과 출입구 접근시간을 실측 행렬로 넣어야 합니다. OTP도 정확한 승차 위치와 역사 내부 보행망이 환승 가능성을 결정한다고 설명합니다. [OTP 역사 내비게이션](https://docs.opentripplanner.org/en/latest/In-Station-Navigation/), [GTFS Pathways](https://gtfs.org/getting-started/features/pathways/)
- 심각도: 🔴 반드시 수정

7. [docs/routing-accuracy.md:26](/C:/Users/SAMSUNG/Desktop/seoul-route/docs/routing-accuracy.md:26) — 서울 bbox 추출 때문에 이미 5,264개 정류장이 도로망과 분리됐습니다.

- 근거: 광역노선은 서울 안의 이동에도 경계 밖 회차·종점과 연결된 완전한 패턴이 필요합니다.
- 영향: 접근·환승 불가, 패턴 절단, 일부 OD의 경로 누락이 발생합니다.
- 수정 제안: 사용자 입력 검증은 서울로 유지하되 OSM 그래프는 모든 채택 노선의 stop/shape envelope에 완충거리를 둬 추출하십시오. `IsolatedStop`은 허용 경고가 아니라 품질 게이트로 관리해야 합니다.
- 심각도: 🟡 고치는 편이 좋음

8. [backend/internal/route/plan.go:109](/C:/Users/SAMSUNG/Desktop/seoul-route/backend/internal/route/plan.go:109) — OTP의 다기준 결과를 최종적으로 소요시간 하나로 다시 정렬합니다.

- 근거: 환승 수, 보행거리, 짧은 환승 성공확률, 따릉이 재고 위험을 무시합니다. 또한 `first=10` 이후 중복 제거라 서로 다른 경로가 10개 확보된다는 보장이 없습니다.
- 영향: 1분 빠르지만 환승 실패 가능성이 높은 경로가 항상 최상단에 놓입니다.
- 수정 제안: 후보는 cursor paging으로 최소 K개의 고유 경로가 생길 때까지 받고, 최종 순위는 `예상시간 + 환승실패 위험 + 환승/보행 불편 + 자전거 재고 위험`으로 정하십시오. UI에는 추천·최단시간·최소환승·최소도보를 분리하는 편이 낫습니다.
- 심각도: 🟡 고치는 편이 좋음

9. [docs/routing-accuracy.md:27](/C:/Users/SAMSUNG/Desktop/seoul-route/docs/routing-accuracy.md:27) — 보정 전에 재현 가능한 품질 기준선이 없습니다.

- 근거: 서울역→강남 사례는 상세하지만, 문서에 적힌 OD 120~150쌍 평가와 ground truth 수집은 아직 실행 체계가 아닙니다.
- 영향: 한 사례를 개선하면서 다른 시간대·권역·노선유형을 악화시켜도 알 수 없습니다.
- 수정 제안: 최소한 다음 축을 층화한 200쌍 이상의 고정 평가셋을 먼저 만드십시오.

  - 평일 첨두·비첨두·토요일·공휴일·막차
  - 버스 중심·지하철 중심·복합환승·따릉이 접근
  - 대형역·한강 횡단·서울 경계·저빈도 노선
  - ETA 중앙절대오차, P90 오차, 실제 가능 경로율, top-3 노선집합 재현율, 환승 성공률

- 심각도: 🟡 고치는 편이 좋음

권장 우선순위는 `평가셋 구축 → 최신 요일별 정적 GTFS → 버스 방향/회차 수정 → 실시간 TripUpdates → 역 출입구·환승망 → 신뢰도 기반 재정렬 → 개인화·횡단보도`입니다.

개인 보행속도와 횡단보도 보정은 좋은 차별점이지만, 현재는 `speed_profiles` 조회만 있고 수집·학습 경로가 노출되어 있지 않습니다. 대중교통 시간표와 실시간성이 안정되기 전에 개인화부터 진행하면 작은 보행 오차를 줄이면서 훨씬 큰 버스·환승 오차를 그대로 남기게 됩니다.

파일은 수정하지 않았고, 이번 평가는 정적 설계·배선 검토입니다. 테스트나 OTP 실기동은 수행하지 않았습니다.
tokens used
166,040
총평: “알고리즘보다 데이터·모델링이 문제”라는 진단은 정확합니다. 다만 현재 변경은 서울역→강남역 한 사례의 시간을 카카오에 맞추는 방향에 가깝고, 실제 운행·환승 가능성을 안정적으로 재현하는 구조는 아직 아닙니다. 현 상태에서 OTP 알고리즘을 바꿔도 카카오 수준에는 도달하기 어렵습니다.

1. `otp/router-config.json:19` — 실시간 대중교통 데이터가 라우팅에 연결되지 않았습니다.

- 근거: updater는 따릉이 `vehicle-rental` 하나뿐입니다. 지금 출발 경로도 버스·지하철 지연, 결행, 실제 도착예정 시간을 사용하지 않습니다.
- 결과: 정적 GTFS가 아무리 정교해도 사고·정체·배차 불규칙이 큰 버스에서 카카오 수준 ETA를 낼 수 없습니다.
- 수정 제안: 서울 버스 도착정보를 GTFS-RT `TripUpdates`로 변환하는 어댑터를 최우선으로 두십시오. 차량 위치는 보조이고, 라우팅에는 정류장별 예측 도착시각이 핵심입니다. 서울 API는 첫·두 번째 차량의 예측 도착시간과 혼잡도까지 제공합니다. OTP도 실시간 TripUpdates를 즉시 라우팅에 적용할 수 있습니다. [서울 버스도착정보](https://www.data.go.kr/dataset/15000314/openapi.do?lang=en), [OTP GTFS-RT 설정](https://docs.opentripplanner.org/en/dev-2.x/GTFS-RT-Config/)
- 심각도: 🔴 반드시 수정

2. [gtfs/internal/build/build.go:197](/C:/Users/SAMSUNG/Desktop/seoul-route/gtfs/internal/build/build.go:197) — 평균 배차간격만으로 `exact_times=1`을 만드는 것은 존재하지 않는 고정 시간표를 발명합니다.

- 근거: `firstBus + n × term`을 하루 종일 정확한 출발시각으로 취급합니다. 그러나 원천 API의 `term`은 고정 시각표가 아니라 대표 배차간격입니다.
- `exact_times=1`은 “실제로 정확히 그 간격으로 운행되는 일정 기반 서비스”에만 쓰는 값입니다. `0`은 비고정 배차 서비스입니다. [GTFS 공식 규격](https://gtfs.org/documentation/schedule/reference/)
- 영향: 특정 출발시각에서는 대기가 우연히 0분이 되고, 몇 분 차이로 노선 순위가 크게 뒤집힙니다. 서울역→강남 시간이 카카오와 가까워진 것은 정확도 향상보다 위상 맞춤일 수 있습니다.
- 수정 제안: 실시간 도착정보가 있으면 그 값을 사용하고, 없으면 시간대별 관측 headway의 기대대기·상위 분위수를 별도 비용으로 모델링하십시오. 실제 출발 로그가 확보됐을 때만 `exact_times=1` 또는 절대 `stop_times`를 생성해야 합니다.
- 심각도: 🔴 반드시 수정

3. [gtfs/internal/build/build.go:200](/C:/Users/SAMSUNG/Desktop/seoul-route/gtfs/internal/build/build.go:200) — 변동하는 `sectSpd` 한 번의 스냅샷과 10분 표본 정차시간을 장기 정적 시간표로 고정합니다.

- 근거: 문서 스스로 `sectSpd`가 70초 사이에도 바뀐다고 기록하면서, 토요일 밤 10분 표본에서 얻은 간선 38초·지선 30초를 모든 정류장과 시간대에 더합니다.
- 영향: 첨두/비첨두, 평일/주말, 중앙차로/가로변, 승하차 수요 차이를 표현하지 못합니다. 오차를 전부 정차시간으로 귀속했기 때문에 신호대기·혼잡·위치 API 양자화까지 중복 가산할 수 있습니다.
- 수정 제안: `노선×방향×구간×요일유형×15~30분 시간대` 단위의 중앙값과 P85 주행시간을 장기간 수집하십시오. 정차시간은 승하차량·정류장 유형과 분리 추정하고, 학습 기간과 다른 날짜를 holdout으로 검증해야 합니다.
- 심각도: 🔴 반드시 수정

4. [docs/gtfs-generator.md:43](/C:/Users/SAMSUNG/Desktop/seoul-route/docs/gtfs-generator.md:43) — 2025년 평일 도시철도 파일럿을 2026년 전 요일에 적용합니다.

- 근거: 버스와 도시철도 모두 `service_id=ALL`이고 월~일이 전부 활성화됩니다. 주말·공휴일 감축 운행과 시간표 개정이 사라집니다.
- 영향: 존재하지 않는 열차를 추천하거나 실제 열차를 놓치며, 막차 경로는 특히 크게 틀립니다.
- 수정 제안: 최신 운영사별 시간표를 평일·토요일·휴일 서비스로 분리하고 `calendar_dates.txt`에 임시 변경과 공휴일을 넣으십시오. 서울교통공사는 이미 세 요일 유형의 역별 시간표를 제공합니다. [서울교통공사 시간표 API](https://www.data.go.kr/data/15056777/openapi.do)
- 심각도: 🔴 반드시 수정

5. [gtfs/internal/seoulbus/types.go:32](/C:/Users/SAMSUNG/Desktop/seoul-route/gtfs/internal/seoulbus/types.go:32) — 회차지 정보가 수집되지만 GTFS 생성에서 무시됩니다.

- 근거: `TransYn`이 존재하는데 전체 `b.Stops`를 하나의 trip으로 만들고, 모든 구간에 동일한 `direction_id=0`과 종점 headsign을 붙입니다.
- 영향: 회차지를 지나 반대 방향까지 하차 없이 계속 타는 경로, 회차 대기 0초 경로, 반대 방향 headsign 오류가 생길 수 있습니다.
- 수정 제안: `TransYn=Y`에서 상·하행 trip을 분리하십시오. 실제 동일 차량 연속운행이 확인되면 `block_id`와 현실적인 회차시간으로 연결해야 합니다.
- 심각도: 🔴 반드시 수정

6. [backend/internal/route/anchor.go:10](/C:/Users/SAMSUNG/Desktop/seoul-route/backend/internal/route/anchor.go:10) — 역 앵커링은 스냅 문제를 해결한 것이 아니라 역 접근시간을 우회합니다.

- 근거: 장소 좌표를 최대 1km 떨어진 이름 기반 부모역으로 바꾸고, 부모역 좌표도 자식 stop의 평균으로 합성합니다. 실제 출입구, 개찰구, 층, 승강장 간 연결은 없습니다.
- 영향: 서울역·고속터미널·왕십리 같은 대형역에서 잘못된 출입구와 불가능한 단거리 환승을 만들 수 있습니다. 54.3→39.8분 감소에는 제거된 우회뿐 아니라 실제 역사 진입시간까지 빠졌을 가능성이 있습니다.
- 수정 제안: 입력 좌표를 유지하고 출입구 후보로 스냅한 뒤 `entrance → pathway → platform`을 모델링하십시오. 당장 어렵다면 `역×노선쌍×방향` 환승시간과 출입구 접근시간을 실측 행렬로 넣어야 합니다. OTP도 정확한 승차 위치와 역사 내부 보행망이 환승 가능성을 결정한다고 설명합니다. [OTP 역사 내비게이션](https://docs.opentripplanner.org/en/latest/In-Station-Navigation/), [GTFS Pathways](https://gtfs.org/getting-started/features/pathways/)
- 심각도: 🔴 반드시 수정

7. [docs/routing-accuracy.md:26](/C:/Users/SAMSUNG/Desktop/seoul-route/docs/routing-accuracy.md:26) — 서울 bbox 추출 때문에 이미 5,264개 정류장이 도로망과 분리됐습니다.

- 근거: 광역노선은 서울 안의 이동에도 경계 밖 회차·종점과 연결된 완전한 패턴이 필요합니다.
- 영향: 접근·환승 불가, 패턴 절단, 일부 OD의 경로 누락이 발생합니다.
- 수정 제안: 사용자 입력 검증은 서울로 유지하되 OSM 그래프는 모든 채택 노선의 stop/shape envelope에 완충거리를 둬 추출하십시오. `IsolatedStop`은 허용 경고가 아니라 품질 게이트로 관리해야 합니다.
- 심각도: 🟡 고치는 편이 좋음

8. [backend/internal/route/plan.go:109](/C:/Users/SAMSUNG/Desktop/seoul-route/backend/internal/route/plan.go:109) — OTP의 다기준 결과를 최종적으로 소요시간 하나로 다시 정렬합니다.

- 근거: 환승 수, 보행거리, 짧은 환승 성공확률, 따릉이 재고 위험을 무시합니다. 또한 `first=10` 이후 중복 제거라 서로 다른 경로가 10개 확보된다는 보장이 없습니다.
- 영향: 1분 빠르지만 환승 실패 가능성이 높은 경로가 항상 최상단에 놓입니다.
- 수정 제안: 후보는 cursor paging으로 최소 K개의 고유 경로가 생길 때까지 받고, 최종 순위는 `예상시간 + 환승실패 위험 + 환승/보행 불편 + 자전거 재고 위험`으로 정하십시오. UI에는 추천·최단시간·최소환승·최소도보를 분리하는 편이 낫습니다.
- 심각도: 🟡 고치는 편이 좋음

9. [docs/routing-accuracy.md:27](/C:/Users/SAMSUNG/Desktop/seoul-route/docs/routing-accuracy.md:27) — 보정 전에 재현 가능한 품질 기준선이 없습니다.

- 근거: 서울역→강남 사례는 상세하지만, 문서에 적힌 OD 120~150쌍 평가와 ground truth 수집은 아직 실행 체계가 아닙니다.
- 영향: 한 사례를 개선하면서 다른 시간대·권역·노선유형을 악화시켜도 알 수 없습니다.
- 수정 제안: 최소한 다음 축을 층화한 200쌍 이상의 고정 평가셋을 먼저 만드십시오.

  - 평일 첨두·비첨두·토요일·공휴일·막차
  - 버스 중심·지하철 중심·복합환승·따릉이 접근
  - 대형역·한강 횡단·서울 경계·저빈도 노선
  - ETA 중앙절대오차, P90 오차, 실제 가능 경로율, top-3 노선집합 재현율, 환승 성공률

- 심각도: 🟡 고치는 편이 좋음

권장 우선순위는 `평가셋 구축 → 최신 요일별 정적 GTFS → 버스 방향/회차 수정 → 실시간 TripUpdates → 역 출입구·환승망 → 신뢰도 기반 재정렬 → 개인화·횡단보도`입니다.

개인 보행속도와 횡단보도 보정은 좋은 차별점이지만, 현재는 `speed_profiles` 조회만 있고 수집·학습 경로가 노출되어 있지 않습니다. 대중교통 시간표와 실시간성이 안정되기 전에 개인화부터 진행하면 작은 보행 오차를 줄이면서 훨씬 큰 버스·환승 오차를 그대로 남기게 됩니다.

파일은 수정하지 않았고, 이번 평가는 정적 설계·배선 검토입니다. 테스트나 OTP 실기동은 수행하지 않았습니다.
