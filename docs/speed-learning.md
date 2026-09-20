# 사용자별 속도 학습

안내 위치 샘플에서 걷기·자전거 이동 속도를 계산해 다음 경로 요청의 OTP 속도에 반영합니다.

## 흐름

1. `POST /trips`로 안내 trip을 만듭니다.
2. 앱이 `POST /trips/{id}/traces`에 위치·정확도·수단·활동 표본을 보냅니다.
3. `POST /trips/{id}/end`가 trip 속도를 계산하고 사용자 프로파일을 갱신합니다.
4. `POST /routes/plan`이 프로파일을 OTP 요청 속도로 변환합니다.

업로드는 `(trip_id, ts)` 중복을 무시하므로 재전송할 수 있습니다.

## 표본 필터

`backend/internal/speed`은 시간순 인접 표본의 거리와 시간으로 속도를 계산합니다.

- 허용 정확도: 50m 이하
- 표본 간격: 2~30초
- 이동 판정: 걷기 0.3m/s 이상, 자전거 0.5m/s 이상
- 최대 속도: 걷기 3.0m/s, 자전거 12.0m/s
- 수단과 활동 인식이 맞지 않는 쌍은 제외
- `still`, `unknown` 활동은 학습에서 제외

수단별 중앙값을 trip 값으로 사용합니다. 사용자 프로파일은 사전값과 trip 중앙값을 수축 결합하며, 걷기 사전값은 1.2m/s, 자전거는 3.5m/s입니다.

## OTP 속도 변환

OTP의 요청 속도는 평지 최대속도이고 GPS 학습값은 실제 이동 평균입니다. 다음 실측 비율로 변환합니다.

| 수단 | 실효 속도 ÷ 요청 속도 | 변환 |
|---|---:|---|
| 걷기 | 0.973 | 평균 ÷ 0.973 |
| 자전거 | 0.89 | 평균 ÷ 0.89 |

도로 데이터나 OTP 버전이 바뀌면 고정 OD 집합에서 `거리 ÷ 소요 ÷ 요청 속도`를 다시 계산합니다.

## 활동 판정

앱은 높은 신뢰도의 걷기·자전거·차량 판정이 20초 유지되면 확정합니다. `still`이 2분 유지되면 확정 활동을 `unknown`으로 되돌립니다. 이 값들은 실제 외출 궤적에서 오분류와 신호 대기를 다시 확인해 조정합니다.

## 보관과 종료

원본 궤적은 30일 후 삭제합니다. 탈퇴하면 trip, trace, 속도 프로파일을 함께 삭제합니다. 24시간 동안 갱신되지 않은 열린 trip은 서버가 종료 처리합니다.

종료 요청이 409를 반환하면 이미 학습까지 끝난 상태로 보고 앱도 완료 처리합니다.

## 검증

```powershell
Set-Location backend
$env:TEST_DATABASE_URL='postgres://seoul:seoul@localhost:5432/seoul_route_test?sslmode=disable'
go test ./internal/speed ./internal/httpapi

Set-Location ../app
flutter test test/trace_uploader_test.dart test/activity_classifier_test.dart test/guide_lifecycle_test.dart
```

실기기 검증에서는 화면을 끈 상태의 업로드, 활동 불일치 제외, 종료 재시도, 프로파일 반영 뒤 경로 응답의 `walk_speed`·`bike_speed`를 확인합니다.
