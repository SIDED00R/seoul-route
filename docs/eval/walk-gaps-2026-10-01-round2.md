# 도보 끊김 후보 (2026-10-01)

`otp/find_walk_gaps.py` 결과. 입력 = 도보 우회로 걸린 쌍 821건.

| 판정 | 쌍 | 뜻 |
|---|---|---|
| fixable | 237 | 막힌 way 다시 열기(R)·막다른 끝 잇기(L) 후보로 설명된다 |
| model | 111 | 후보 없이도 OTP 보다 짧다 — OTP 쪽 제약(보행 안전 가중치·private 통과 제한) |
| unexplained | 470 | 하천·철도 같은 실제 장벽이거나 영역 밖 문제 |
| snap | 3 | 출발·도착 150m 안에 보행 길이 없다 |

## 후보 (쌍 수 × 최대 절감 순, 최대 60건)

`단독` = 그 후보 하나로 설명된 쌍 수. TMap 열은 걸린 쌍에 TMap 경로선이 있을 때만 찬다(지남/안 지남).
등급: R = 막힌 way 다시 열기(사람 확인), A = 20m 이하이고 큰길을 안 건너는 잇기, B = 그 밖의 잇기.

| 등급 | OSM | 이름·태그 | 길이(m) | 편집일 | 쌍 | 단독 | 최대 절감(m) | TMap | 위치 |
|---|---|---|---|---|---|---|---|---|---|
| R | [way 476599351](https://www.openstreetmap.org/way/476599351) | cycleway  foot=- access=- (보행 표시 없는 자전거길) | 53 | 2024-09-13 | 10 | 10 | 1435 | - | 37.667519,127.050597 |
| B | [node 11364090157](https://www.openstreetmap.org/node/11364090157) → [node 9013250259](https://www.openstreetmap.org/node/9013250259) | way 1225418999 끝 → way 408640145 (큰길 가로지름) | 57 |  | 8 | 7 | 1494 | - | 37.630842,126.813471 |
| B | [node 11822700900](https://www.openstreetmap.org/node/11822700900) → [node 3369577593](https://www.openstreetmap.org/node/3369577593) | way 1273156139 끝 → way 656122981 | 36 |  | 8 | 7 | 1426 | - | 37.567287,126.987377 |
| B | [node 11231375504](https://www.openstreetmap.org/node/11231375504) → [node 3848592668](https://www.openstreetmap.org/node/3848592668) | way 1212265720 끝 → way 381695287 | 44 |  | 5 | 5 | 1593 | - | 37.39844,126.984416 |
| B | [node 11063493990](https://www.openstreetmap.org/node/11063493990) → [node 11009372937](https://www.openstreetmap.org/node/11009372937) | way 361724818 끝 → way 468588632,1185175224 | 30 |  | 4 | 2 | 1407 | - | 37.450808,126.75598 |
| R | [way 505438094](https://www.openstreetmap.org/way/505438094) | cycleway  foot=- access=- (보행 표시 없는 자전거길) | 67 | 2017-07-06 | 4 | 4 | 1372 | - | 37.632341,127.039469 |
| B | [node 12996619963](https://www.openstreetmap.org/node/12996619963) → [node 4822116743](https://www.openstreetmap.org/node/4822116743) | way 1414352224 끝 → way 623419820 | 21 |  | 2 | 2 | 2423 | - | 37.612381,127.110283 |
| B | [node 13588260795](https://www.openstreetmap.org/node/13588260795) → [node 13680915507](https://www.openstreetmap.org/node/13680915507) | way 1481585303 끝 → way 1492976957 | 23 |  | 3 | 3 | 1518 | - | 37.465737,127.101417 |
| B | [node 4874729811](https://www.openstreetmap.org/node/4874729811) → [node 1327540538](https://www.openstreetmap.org/node/1327540538) | way 495738196 끝 → way 117940271,226939721 (큰길 가로지름) | 57 |  | 4 | 4 | 1099 | - | 37.563992,126.806237 |
| A | [node 4672280763](https://www.openstreetmap.org/node/4672280763) → [node 4030832248](https://www.openstreetmap.org/node/4030832248) | way 473057556 끝 → way 400533478 | 20 |  | 4 | 3 | 1070 | - | 37.57251,126.899412 |
| B | [node 13680915627](https://www.openstreetmap.org/node/13680915627) → [node 13680915593](https://www.openstreetmap.org/node/13680915593) | way 1492977018 끝 → way 1492976969 | 32 |  | 6 | 5 | 705 | - | 37.456918,127.059397 |
| R | [way 531415240](https://www.openstreetmap.org/way/531415240) | cycleway 성내천 자전거길 foot=- access=- (보행 표시 없는 자전거길) | 1126 | 2020-05-16 | 3 | 3 | 1267 | - | 37.52331,127.115033 |
| B | [node 10734586462](https://www.openstreetmap.org/node/10734586462) → [node 5363819138](https://www.openstreetmap.org/node/5363819138) | way 1154228711 끝 → way 556025391 | 32 |  | 3 | 2 | 1262 | - | 37.72553,126.766415 |
| B | [node 10271456277](https://www.openstreetmap.org/node/10271456277) → [node 4586354677](https://www.openstreetmap.org/node/4586354677) | way 1114769661 끝 → way 220707251,388547125 | 32 |  | 2 | 2 | 1864 | - | 37.600975,126.95928 |
| B | [node 7127776131](https://www.openstreetmap.org/node/7127776131) → [node 9218140319](https://www.openstreetmap.org/node/9218140319) | way 762789481 끝 → way 998361624,1011423040 | 47 |  | 3 | 3 | 1221 | - | 37.661153,126.742335 |
| A | [node 6339730934](https://www.openstreetmap.org/node/6339730934) → [node 436883043](https://www.openstreetmap.org/node/436883043) | way 677040046 끝 → way 170277491,473309761,473309762 | 15 |  | 3 | 3 | 1151 | - | 37.532905,126.963143 |
| R | [way 892260899](https://www.openstreetmap.org/way/892260899) | construction  foot=- access=- | 1118 | 2026-07-05 | 3 | 3 | 1061 | - | 37.634494,127.112645 |
| B | [node 13797835550](https://www.openstreetmap.org/node/13797835550) → [node 2534588052](https://www.openstreetmap.org/node/2534588052) | way 795858155 끝 → way 516166425 (큰길 가로지름) | 39 |  | 5 | 0 | 636 | - | 37.536867,126.970446 |
| A | [node 6385354974](https://www.openstreetmap.org/node/6385354974) → [node 6385356987](https://www.openstreetmap.org/node/6385356987) | way 681836504 끝 → way 733103044 | 11 |  | 1 | 0 | 2940 | - | 37.730576,127.091715 |
| B | [node 6385354974](https://www.openstreetmap.org/node/6385354974) → [node 11248224609](https://www.openstreetmap.org/node/11248224609) | way 681836504 끝 → way 472494850 | 32 |  | 1 | 0 | 2940 | - | 37.73053,127.091486 |
| B | [node 4634465392](https://www.openstreetmap.org/node/4634465392) → [node 13016597872](https://www.openstreetmap.org/node/13016597872) | way 469030123 끝 → way 1317598611,1416453746 | 25 |  | 3 | 0 | 971 | - | 37.50716,126.742019 |
| A | [node 13797835568](https://www.openstreetmap.org/node/13797835568) → [node 13797835575](https://www.openstreetmap.org/node/13797835575) | way 795858157 끝 → way 1263157786 | 20 |  | 5 | 0 | 554 | - | 37.536449,126.971531 |
| B | [node 4450237019](https://www.openstreetmap.org/node/4450237019) → [node 7257403436](https://www.openstreetmap.org/node/7257403436) | way 447965325 끝 → way 777660085 | 40 |  | 3 | 3 | 885 | - | 37.572246,126.959499 |
| B | [node 8357405527](https://www.openstreetmap.org/node/8357405527) → [node 11063493964](https://www.openstreetmap.org/node/11063493964) | way 899473332 끝 → way 1191620569 | 32 |  | 2 | 0 | 1288 | - | 37.449557,126.749963 |
| B | [node 11822700900](https://www.openstreetmap.org/node/11822700900) → [node 8486350168](https://www.openstreetmap.org/node/8486350168) | way 1273156139 끝 → way 228795478,913565824 (큰길 가로지름) | 46 |  | 2 | 2 | 1225 | - | 37.567291,126.987461 |
| R | [way 713929100](https://www.openstreetmap.org/way/713929100) | cycleway  foot=- access=- (보행 표시 없는 자전거길) | 2 | 2025-08-02 | 2 | 0 | 1102 | - | 37.696286,127.047996 |
| R | [way 713929102](https://www.openstreetmap.org/way/713929102) | cycleway  foot=- access=- (보행 표시 없는 자전거길) | 26 | 2025-08-02 | 2 | 0 | 1102 | - | 37.696228,127.049486 |
| R | [way 713929101](https://www.openstreetmap.org/way/713929101) | cycleway  foot=- access=- (보행 표시 없는 자전거길) | 105 | 2025-08-02 | 2 | 0 | 1102 | - | 37.696251,127.049189 |
| A | [node 13051081719](https://www.openstreetmap.org/node/13051081719) → [node 7984044035](https://www.openstreetmap.org/node/7984044035) | way 1420285930 끝 → way 472070749 | 15 |  | 3 | 0 | 673 | - | 37.557431,126.808757 |
| A | [node 13051081719](https://www.openstreetmap.org/node/13051081719) → [node 2290159211](https://www.openstreetmap.org/node/2290159211) | way 1420285930 끝 → way 220313737 | 18 |  | 3 | 0 | 673 | - | 37.557526,126.808899 |
| R | [way 301354088](https://www.openstreetmap.org/way/301354088) | residential 녹사평대로 foot=no access=- | 311 | 2019-07-02 | 2 | 2 | 921 | - | 37.525232,126.995223 |
| B | [node 12032119300](https://www.openstreetmap.org/node/12032119300) → [node 9327502416](https://www.openstreetmap.org/node/9327502416) | way 1298891610 끝 → way 1011006725 | 35 |  | 3 | 3 | 595 | - | 37.477234,127.114012 |
| B | [node 9052183231](https://www.openstreetmap.org/node/9052183231) → [node 2605603669](https://www.openstreetmap.org/node/2605603669) | way 978283533 끝 → way 515850851,515850853,515850856 | 38 |  | 4 | 4 | 431 | - | 37.580549,126.893223 |
| B | [node 2615867623](https://www.openstreetmap.org/node/2615867623) → [node 2812775450](https://www.openstreetmap.org/node/2812775450) | way 255871727 끝 → way 276687111,276920359 | 22 |  | 2 | 1 | 855 | - | 37.520175,126.882128 |
| A | [node 10679772932](https://www.openstreetmap.org/node/10679772932) → [node 4104386589](https://www.openstreetmap.org/node/4104386589) | way 1147803884 끝 → way 408535339 | 16 |  | 2 | 2 | 848 | - | 37.538455,126.986664 |
| A | [node 13051081717](https://www.openstreetmap.org/node/13051081717) → [node 7984044019](https://www.openstreetmap.org/node/7984044019) | way 1420285929 끝 → way 472070755 | 17 |  | 2 | 0 | 786 | - | 37.557201,126.809102 |
| B | [node 13051081717](https://www.openstreetmap.org/node/13051081717) → [node 2290159211](https://www.openstreetmap.org/node/2290159211) | way 1420285929 끝 → way 220313737 | 37 |  | 2 | 0 | 786 | - | 37.557425,126.809058 |
| B | [node 8457996789](https://www.openstreetmap.org/node/8457996789) → [node 8457322992](https://www.openstreetmap.org/node/8457322992) | way 910863952 끝 → way 910713003,910806121 | 49 |  | 2 | 2 | 743 | - | 37.582798,126.881723 |
| B | [node 13358779167](https://www.openstreetmap.org/node/13358779167) → [node 4643770718](https://www.openstreetmap.org/node/4643770718) | way 1527148102 끝 → way 70998427,470150463 (큰길 가로지름) | 49 |  | 2 | 2 | 732 | - | 37.49576,127.070241 |
| B | [node 3401123578](https://www.openstreetmap.org/node/3401123578) → [node 2244930341](https://www.openstreetmap.org/node/2244930341) | way 1384270205 끝 → way 919393794,931397909 | 25 |  | 3 | 3 | 485 | - | 37.502824,126.980746 |
| B | [node 9160179481](https://www.openstreetmap.org/node/9160179481) → [node 4658265298](https://www.openstreetmap.org/node/4658265298) | way 991429295 끝 → way 471666461,1225418997 | 21 |  | 1 | 0 | 1371 | - | 37.627137,126.815949 |
| R | [way 478139613](https://www.openstreetmap.org/way/478139613) | cycleway  foot=- access=- (보행 표시 없는 자전거길) | 15 | 2026-06-24 | 3 | 0 | 451 | - | 37.450433,127.003822 |
| R | [way 816075675](https://www.openstreetmap.org/way/816075675) | cycleway  foot=- access=- (보행 표시 없는 자전거길) | 58 | 2026-06-24 | 3 | 0 | 451 | - | 37.450282,127.003712 |
| R | [way 478139615](https://www.openstreetmap.org/way/478139615) | cycleway  foot=- access=- (보행 표시 없는 자전거길) | 253 | 2024-10-18 | 3 | 0 | 451 | - | 37.451473,127.003722 |
| B | [node 12872559406](https://www.openstreetmap.org/node/12872559406) → [node 2915620447](https://www.openstreetmap.org/node/2915620447) | way 1390626139 끝 → way 707995071 | 20 |  | 2 | 2 | 641 | - | 37.438483,127.145656 |
| A | [node 4699168684](https://www.openstreetmap.org/node/4699168684) → [node 4698319396](https://www.openstreetmap.org/node/4698319396) | way 476308295 끝 → way 302746646 | 12 |  | 1 | 0 | 1262 | - | 37.724584,126.75828 |
| A | [node 3849370229](https://www.openstreetmap.org/node/3849370229) → [node 8959033082](https://www.openstreetmap.org/node/8959033082) | way 381769408 끝 → way 82088515 | 18 |  | 2 | 1 | 628 | - | 37.626884,127.088787 |
| B | [node 12876530381](https://www.openstreetmap.org/node/12876530381) → [node 4634465314](https://www.openstreetmap.org/node/4634465314) | way 1391094817 끝 → way 469030101 (큰길 가로지름) | 40 |  | 2 | 2 | 606 | - | 37.508456,126.738276 |
| B | [node 8540948770](https://www.openstreetmap.org/node/8540948770) → [node 11228938198](https://www.openstreetmap.org/node/11228938198) | way 919711812 끝 → way 1212049158 (큰길 가로지름) | 20 |  | 2 | 2 | 594 | - | 37.522339,126.836673 |
| A | [node 11828193095](https://www.openstreetmap.org/node/11828193095) → [node 4618085845](https://www.openstreetmap.org/node/4618085845) | way 1200964127 끝 → way 437912695 | 17 |  | 1 | 1 | 1162 | - | 37.377491,127.110243 |
| A | [node 7558478367](https://www.openstreetmap.org/node/7558478367) → [node 436839328](https://www.openstreetmap.org/node/436839328) | way 808311364 끝 → way 712133750 | 12 |  | 2 | 0 | 573 | - | 37.677329,127.049036 |
| R | [way 1346992505](https://www.openstreetmap.org/way/1346992505) | residential 장한로10길 foot=no access=- | 14 | 2024-12-29 | 1 | 1 | 1146 | - | 37.565412,127.067448 |
| B | [node 11850445867](https://www.openstreetmap.org/node/11850445867) → [node 6586520023](https://www.openstreetmap.org/node/6586520023) | way 1276438325 끝 → way 852524681 | 30 |  | 2 | 0 | 573 | - | 37.677358,127.049578 |
| B | [node 7558478367](https://www.openstreetmap.org/node/7558478367) → [node 11850445867](https://www.openstreetmap.org/node/11850445867) | way 808311364 끝 → way 1276438325 | 40 |  | 2 | 0 | 573 | - | 37.677384,127.04929 |
| B | [node 8488404298](https://www.openstreetmap.org/node/8488404298) → [node 1918413966](https://www.openstreetmap.org/node/1918413966) | way 913786064 끝 → way 517761241 | 42 |  | 2 | 2 | 557 | - | 37.580324,127.010825 |
| R | [way 632418566](https://www.openstreetmap.org/way/632418566) | cycleway  foot=- access=- (보행 표시 없는 자전거길) | 121 | 2020-06-12 | 1 | 0 | 1102 | - | 37.690974,127.049322 |
| B | [node 13357273161](https://www.openstreetmap.org/node/13357273161) → [node 3740141144](https://www.openstreetmap.org/node/3740141144) | way 1456459312 끝 → way 183272296,370304353 | 33 |  | 2 | 2 | 543 | - | 37.601693,127.076391 |
| R | [way 713929097](https://www.openstreetmap.org/way/713929097) | cycleway  foot=- access=- (보행 표시 없는 자전거길) | 212 | 2020-02-02 | 1 | 0 | 1023 | - | 37.695849,127.047914 |
| B | [node 13366760918](https://www.openstreetmap.org/node/13366760918) → [node 3828439712](https://www.openstreetmap.org/node/3828439712) | way 1457457515 끝 → way 379533752 (큰길 가로지름) | 58 |  | 2 | 2 | 510 | - | 37.490459,127.080964 |
| A | [node 13003882363](https://www.openstreetmap.org/node/13003882363) → [node 11034453335](https://www.openstreetmap.org/node/11034453335) | way 1415118272 끝 → way 1188315029 | 16 |  | 1 | 1 | 977 | - | 37.479766,127.1066 |

## model 판정 쌍이 지난 private way (많은 순, 최대 20건)

| way | 쌍 |
|---|---|
| [693480772](https://www.openstreetmap.org/way/693480772) | 5 |
| [681836498](https://www.openstreetmap.org/way/681836498) | 3 |
| [692770111](https://www.openstreetmap.org/way/692770111) | 3 |
| [691174966](https://www.openstreetmap.org/way/691174966) | 3 |
| [691174981](https://www.openstreetmap.org/way/691174981) | 3 |
| [1092520705](https://www.openstreetmap.org/way/1092520705) | 2 |
| [692770112](https://www.openstreetmap.org/way/692770112) | 2 |
| [692770109](https://www.openstreetmap.org/way/692770109) | 2 |
| [692770121](https://www.openstreetmap.org/way/692770121) | 2 |
| [692770108](https://www.openstreetmap.org/way/692770108) | 2 |
| [691912361](https://www.openstreetmap.org/way/691912361) | 2 |
| [691912360](https://www.openstreetmap.org/way/691912360) | 2 |
| [691912351](https://www.openstreetmap.org/way/691912351) | 2 |
| [380863301](https://www.openstreetmap.org/way/380863301) | 1 |
| [1224608687](https://www.openstreetmap.org/way/1224608687) | 1 |
| [691912340](https://www.openstreetmap.org/way/691912340) | 1 |
| [886405234](https://www.openstreetmap.org/way/886405234) | 1 |
