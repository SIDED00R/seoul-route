# 횡단보도 신호 대기

도보·따릉이 구간의 예상시간에 신호 횡단보도 기대 대기를 더합니다.

## 데이터

`python otp/extract_crossings.py`가 서울 OSM에서 다음 노드를 `otp/data/crossings.csv`로 만듭니다.

- 신호 태그가 있는 `highway=crossing`
- trunk·primary·secondary·tertiary 도로 위의 무태그 횡단 노드

백엔드는 기동 시 CSV를 공간 색인으로 읽습니다. 파일이 없으면 기능을 끄고 경고를 남깁니다.

## 계산

`backend/internal/crossing`은 leg 폴리라인 주변의 횡단보도를 찾고 15m 안의 중복 노드를 합칩니다. 기본 신호 주기 130초, 보행 녹색 30초에서 무조건부 기대 대기는 다음과 같습니다.

```text
red² / (2 × cycle) = 100² / 260 ≈ 38초
```

역 안 환승 통로는 횡단보도를 세지 않습니다. 대기 때문에 다음 탑승을 놓치면 해당 정류장에서 뒤 구간을 한 번 다시 탐색합니다.

응답은 leg의 `crossings`, `crossing_wait_sec`와 itinerary의 합계 `crossing_wait_sec`, 재탐색 여부 `replanned`를 포함합니다.

## 재보정

38초는 전 횡단보도 공통 초기값입니다. 실제 궤적에서 횡단 전후 정지 시간을 분리할 수 있을 때 도로 유형·시간대별 값으로 교체합니다. T-data 잔여시간 API는 호출 제한과 좌표 부재 때문에 요청 시점 실시간 보정에는 사용하지 않습니다.
