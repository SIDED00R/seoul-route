# OSM 보행 폐쇄 표시 조사 (2026-10-01)

`otp/scan_closed_ways.py` 결과. 전체 1820건(way 기준, 한 way 가 여러 사유에 걸릴 수 있다).

| 사유 | 건수 |
|---|---|
| `highway=construction` | 776 |
| `foot=no` | 641 |
| `access=no` | 379 |
| `name~` | 15 |
| `note~` | 10 |
| `description~` | 8 |
| `name:ko~` | 1 |

## 이름·메모에 폐쇄 표시가 있는 way (오래된 편집 순, 최대 60건)

| way | 분류 | highway | 이름 | 사유 | 편집일 | 길이(m) | 위치 |
|---|---|---|---|---|---|---|---|
| [327214342](https://www.openstreetmap.org/way/327214342) | walk | residential | 북아현로11라길 | note~construction | 2019-07-06 | 29 | 37.561271,126.952215 |
| [727839893](https://www.openstreetmap.org/way/727839893) | blocked | construction |  | highway=construction;note~공사 | 2020-04-07 | 158 | 37.460767,127.053451 |
| [633535411](https://www.openstreetmap.org/way/633535411) | walk | path | 한강 자전거길 (폐쇄) | name~폐쇄 | 2021-09-14 | 615 | 37.544622,126.891928 |
| [983164899](https://www.openstreetmap.org/way/983164899) | walk | path | 한강 자전거길(폐쇄) | name~폐쇄;name:ko~폐쇄 | 2021-09-19 | 75 | 37.546406,126.890418 |
| [1019348677](https://www.openstreetmap.org/way/1019348677) | walk | footway |  | description~공사 | 2022-01-08 | 20 | 37.491773,126.871782 |
| [255872466](https://www.openstreetmap.org/way/255872466) | walk | footway |  | description~공사 | 2022-04-05 | 310 | 37.490681,126.871987 |
| [566499354](https://www.openstreetmap.org/way/566499354) | walk | footway |  | description~공사 | 2022-04-05 | 192 | 37.491897,126.871906 |
| [1057563420](https://www.openstreetmap.org/way/1057563420) | cycle | cycleway | 제2팔당대교 공사 우회 자전거도로 | name~공사 | 2022-05-05 | 118 | 37.54306,127.237019 |
| [1117562602](https://www.openstreetmap.org/way/1117562602) | blocked | construction | 광남로(공사중단) | highway=construction;name~공사 | 2022-11-27 | 145 | 37.45738,126.840033 |
| [1177445882](https://www.openstreetmap.org/way/1177445882) | blocked | construction | 남부순환도로 연결길 공사중. | highway=construction;name~공사 | 2023-05-30 | 9 | 37.492466,126.868005 |
| [1057563419](https://www.openstreetmap.org/way/1057563419) | blocked | construction | 한강남자전거길 | highway=construction;note~공사 | 2025-05-06 | 123 | 37.543147,127.237422 |
| [1004279077](https://www.openstreetmap.org/way/1004279077) | walk | residential | 양평로27길 | description~공사 | 2025-06-20 | 95 | 37.536582,126.89139 |
| [1018182320](https://www.openstreetmap.org/way/1018182320) | walk | footway |  | description~공사 | 2025-07-19 | 200 | 37.491999,126.871097 |
| [816366005](https://www.openstreetmap.org/way/816366005) | blocked | construction | 남부순환도로 연결길 공사중. | highway=construction;name~공사 | 2025-07-19 | 7 | 37.492381,126.868007 |
| [1416381623](https://www.openstreetmap.org/way/1416381623) | walk | footway | 산곡초 임시통학로 | name~임시 | 2025-07-21 | 98 | 37.507746,126.701707 |
| [1444099088](https://www.openstreetmap.org/way/1444099088) | cycle | cycleway | 여의도공원 자전거길 | note~construction | 2025-10-23 | 238 | 37.524676,126.921072 |
| [1444112264](https://www.openstreetmap.org/way/1444112264) | walk | footway | 여의도공원 산책길 | note~closed | 2025-10-23 | 42 | 37.524352,126.920918 |
| [1478779798](https://www.openstreetmap.org/way/1478779798) | blocked | footway |  | access=no;note~Closed | 2026-02-17 | 19 | 37.630652,127.063893 |
| [566953974](https://www.openstreetmap.org/way/566953974) | blocked | steps | 경춘철교 엘리베이터 | access=no;note~Closed | 2026-02-17 | 17 | 37.630592,127.063868 |
| [1097751900](https://www.openstreetmap.org/way/1097751900) | blocked | footway |  | access=no;note~Closed | 2026-02-17 | 16 | 37.630562,127.063905 |
| [46623283](https://www.openstreetmap.org/way/46623283) | walk | trunk | 경부고속도로 | description~공사 | 2026-05-03 | 20 | 37.520298,127.017837 |
| [46623300](https://www.openstreetmap.org/way/46623300) | walk | trunk_link | 경부고속도로 | description~공사 | 2026-05-17 | 154 | 37.52124,127.018192 |
| [46623279](https://www.openstreetmap.org/way/46623279) | walk | trunk | 경부고속도로 | description~공사 | 2026-05-17 | 91 | 37.519935,127.018392 |
| [1148085121](https://www.openstreetmap.org/way/1148085121) | walk | path |  | note~통제 | 2026-06-09 | 450 | 37.750754,127.139938 |
| [1148085094](https://www.openstreetmap.org/way/1148085094) | walk | path |  | note~통제 | 2026-06-09 | 297 | 37.748854,127.138268 |
| [1548147084](https://www.openstreetmap.org/way/1548147084) | walk | secondary | 수내교 (임시교량-분당방면) | name~임시 | 2026-08-08 | 262 | 37.382153,127.115613 |
| [1548147086](https://www.openstreetmap.org/way/1548147086) | walk | secondary | 수내교(공사중-분당방면) | name~공사 | 2026-08-08 | 180 | 37.382041,127.116112 |
| [1554167820](https://www.openstreetmap.org/way/1554167820) | walk | steps | 보수공사중 | name~공사 | 2026-09-01 | 80 | 37.593809,126.971154 |
| [1554061808](https://www.openstreetmap.org/way/1554061808) | walk | steps | 위험지역 폐쇄함 | name~폐쇄 | 2026-09-01 | 24 | 37.587001,126.961945 |
| [1554167848](https://www.openstreetmap.org/way/1554167848) | walk | path | 폐쇄 | name~폐쇄 | 2026-09-01 | 13 | 37.593997,126.976747 |
| [1554167844](https://www.openstreetmap.org/way/1554167844) | walk | steps | 폐쇄 | name~폐쇄 | 2026-09-01 | 12 | 37.593643,126.976007 |
| [1554167823](https://www.openstreetmap.org/way/1554167823) | walk | path | 보수공사중 | name~공사 | 2026-09-01 | 9 | 37.593895,126.970831 |
| [1556489296](https://www.openstreetmap.org/way/1556489296) | blocked | path | 임시 폐쇠 | access=no;name~임시 | 2026-09-06 | 257 | 37.597584,126.97654 |
