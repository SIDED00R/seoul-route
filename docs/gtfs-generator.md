# 운영 GTFS 생성기

`gtfs/`는 서울 버스 API와 도시철도 시간표를 하나의 GTFS zip으로 만듭니다.

## 입력

| 입력 | 생성 방법 | 용도 |
|---|---|---|
| 서울 버스 노선·정류장·경로 | `go run ./cmd/gtfsgen fetch` | 버스 routes, stops, stop_times, frequencies, shapes |
| 국가교통DB GTFS 파일럿 | `otp/data/202503_GTFS_DataSet/` | 도시철도 역·노선 기준 |
| 서울교통공사 시간표 | `python otp/fetch_metro_timetable.py` | 1~9호선 평일·토·일 시간표 |
| 레일포털 시간표 | `python otp/fetch_kric_timetable.py` | 코레일·민자 노선 시간표 |
| OSM 출입구 | `python otp/extract_entrances.py` | 역 출입구와 승강장 pathway |
| OSM 선로 | `python otp/extract_rail.py` | 도시철도 shapes |

## 생성

```powershell
Set-Location gtfs
go run ./cmd/gtfsgen fetch
go run ./cmd/gtfsgen build
Copy-Item out/seoul-gtfs.zip ../otp/data/seoul-gtfs.zip
```

`fetch`는 원본 응답을 `gtfs/cache/`에 보관합니다. `build`는 `gtfs/out/seoul-gtfs.zip`을 생성합니다.

## 변환 규칙

### 버스

- 회차 정류장에서 상·하행 trip을 나눕니다.
- 서울 OSM bbox 안의 정류장만 GTFS에 포함하되, 누적 주행시간은 잘린 바깥 구간까지 유지합니다.
- `sectSpd`가 없으면 18km/h를 사용합니다.
- 정차시간은 간선 38초, 지선·기타 30초, 공항·마을·광역 0초입니다.
- 첫차·막차와 대표 배차간격으로 frequency trip과 막차 trip을 만듭니다.
- 노선 경로가 정류장과 맞으면 `shapes.txt`를 만들고, 맞지 않으면 해당 방향은 직선으로 둡니다.

버스 시간은 실제 출발 시각표가 아니라 평균 배차와 구간 속도를 이용한 근사입니다.

### 도시철도

- 1~9호선은 서울교통공사 시간표가 있으면 파일럿 trip을 대체합니다.
- 경의중앙·수인분당·경춘·경강·공항철도·신분당 등은 레일포털 시간표가 있으면 대체합니다.
- 대체 시간표가 없거나 파일럿 역과 연결되지 않는 구간은 파일럿 데이터를 유지합니다.
- 평일·토요일·일요일 service를 분리합니다.
- 도시철도 shape은 OSM 선로를 따라 만들고, 선로 연결을 찾지 못한 hop은 직선으로 잇습니다.

### 역과 출입구

같은 기준명이고 800m 안인 승강장을 부모역으로 묶습니다. OSM 출입구는 가장 가까운 부모역과 500m 안 승강장에 연결합니다. 실제 출입구가 없는 다승강장 역은 승강장 좌표 출입구를 사용합니다.

승강장 간 pathway는 원본 환승시간을 우선하고, 값이 없으면 거리와 역 구내 보행 상수로 계산합니다.

## 검증

```powershell
Set-Location gtfs
go vet ./...
go test ./...
go run ./cmd/gtfsgen build
```

빌드 보고서에서 다음 항목을 확인합니다.

- 빠진 버스 노선·방향과 shape 실패 수
- 부모역이 달라 연결하지 못한 환승
- 출입구와 연결되지 않은 승강장
- 시각 역행 또는 정차 2개 미만으로 빠진 열차
- 선로를 찾지 못해 직선으로 이은 도시철도 구간

GTFS를 바꾼 뒤에는 OTP 그래프를 다시 빌드하고 `Transit built. |Stops|=`가 0이 아닌지 확인합니다.

## 재보정

버스 속도·정차시간, 역 구내 보행시간, 출입구 연결 반경은 원천 데이터나 OTP 버전이 바뀌면 다시 측정합니다. 현재 수치의 측정 기준은 각 상수 주석과 테스트에 남깁니다.
