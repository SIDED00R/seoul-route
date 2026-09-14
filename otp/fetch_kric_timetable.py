"""레일포털(KRIC) Open API 로 코레일·민자 노선의 요일별 열차 시각표를 내려받아 CSV 두 개로 저장한다.

사용: python otp/fetch_kric_timetable.py → otp/data/kric-stations.csv, otp/data/kric-timetable.csv
GTFS 생성기(gtfs/internal/kric)가 이 두 파일로 아래 노선의 trip 을 만든다(파일 없으면 국가교통DB 파일럿 trip 그대로).
키는 .env 의 KRIC_API_KEY(레일포털 회원가입 후 발급, 승인 API: 도시철도 전체노선정보·역사별 정보·역사별 운행시각표).

노선별 호출: subwayRouteInfo(역 순서) 1회 + stationInfo(좌표) 1회 + stationTimetable(역별·요일별 8 평일/7 토/9 휴일) 역 수×3.
2026-09-14 실측: 코레일·공항철도·신분당 등은 토요일(7) 응답이 0건이고 휴일(9) 시각표를 토요일에도 쓴다(우이신설만 토요일 별도).
GTX-A 는 레일포털에 없고, 서해선은 소사~원시 12역만 있어(대곡 연장 구간 없음) 둘 다 파일럿을 유지한다.
"""

import csv
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

BASE = "https://openapi.kric.go.kr/openapi/"
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT_STATIONS = os.path.join(ROOT, "otp", "data", "kric-stations.csv")
OUT_TIMETABLE = os.path.join(ROOT, "otp", "data", "kric-timetable.csv")
PAUSE = 0.15  # 초. 레일포털은 "과도한 트래픽 시 제한" 만 명시(한도 미공개)

# (파일럿 route_id 의 노선 코드 RR_ACC1_S-1-<code>-…, 레일포털 운영기관 코드, 선 코드)
TARGETS = [
    ("KJ", "KR", "K4"),  # 경의중앙
    ("SD", "KR", "K1"),  # 수인분당
    ("GC", "KR", "K2"),  # 경춘
    ("KK", "KR", "K5"),  # 경강
    ("AP", "AR", "A1"),  # 공항철도
    ("SB", "DX", "D1"),  # 신분당
    ("UI", "UL", "U1"),  # 의정부경전철
    ("SL", "SL", "L1"),  # 신림선
    ("WS", "UI", "UI"),  # 우이신설
    ("KP", "GM", "G1"),  # 김포골드라인
    ("I1", "IC", "I1"),  # 인천1호선
    ("I2", "IC", "I2"),  # 인천2호선
]
DAYS = ("8", "7", "9")


def env_key() -> str:
    with open(os.path.join(ROOT, ".env"), encoding="utf-8") as f:
        for line in f:
            if line.startswith("KRIC_API_KEY="):
                v = line.split("=", 1)[1].strip()
                if v:
                    return v
    print("KRIC_API_KEY 가 .env 에 없다", file=sys.stderr)
    sys.exit(1)


def call(key: str, path: str, **params) -> list:
    params.update(serviceKey=key, format="json")
    url = BASE + path + "?" + urllib.parse.urlencode(params)
    shown = {k: v for k, v in params.items() if k != "serviceKey"}
    for attempt in range(3):
        try:
            with urllib.request.urlopen(url, timeout=60) as r:
                j = json.loads(r.read().decode("utf-8"))
            # 오류도 HTTP 200 으로 온다(실측: 잘못된 키 = resultCode "30", body 없음). "00" 성공, "03" 데이터 없음(토요일
            # 시각표 없는 노선·도라산 등 정상적인 빈 결과)만 받고 그 외는 실패로 본다 — 빈 결과로 저장하면 그 노선의
            # 파일럿 trip 이 전부 사라진 채 시각표 trip 0 인 GTFS 가 나온다.
            header = j.get("header") or {}
            code = str(header.get("resultCode", ""))
            if code == "03":
                return []
            if code == "00":
                body = j.get("body")
                return body if isinstance(body, list) else []
            raise RuntimeError(f"resultCode {code} {header.get('resultMsg', '')}")
        except (urllib.error.URLError, json.JSONDecodeError, TimeoutError, RuntimeError) as e:
            msg = str(e).replace(key, "***")
            if attempt == 2:
                raise RuntimeError(f"{path} {shown}: {msg}")
            time.sleep(2)
    return []


def main() -> int:
    key = env_key()
    stations_rows = []
    tt_rows = []
    for pilot, opr, ln in TARGETS:
        t0 = time.time()
        route = [s for s in call(key, "trainUseInfo/subwayRouteInfo", mreaWideCd="01", lnCd=ln)
                 if s.get("railOprIsttCd") == opr]
        route.sort(key=lambda s: int(s["stinConsOrdr"]))
        coords = {s["stinCd"]: s for s in call(key, "convenientInfo/stationInfo", railOprIsttCd=opr, lnCd=ln)}
        name = route[0]["routNm"] if route else ""
        n_rows = 0
        for s in route:
            c = coords.get(s["stinCd"], {})
            stations_rows.append([pilot, opr, ln, name, s["stinCd"], s["stinConsOrdr"], s["stinNm"].strip(),
                                  c.get("stinLocLat") or "", c.get("stinLocLon") or ""])
            for day in DAYS:
                for r in call(key, "convenientInfo/stationTimetable", railOprIsttCd=opr, lnCd=ln,
                              stinCd=s["stinCd"], dayCd=day):
                    tt_rows.append([pilot, day, r["trnNo"], s["stinCd"], r.get("arvTm") or "", r.get("dptTm") or "",
                                    r.get("orgStinCd") or "", r.get("tmnStinCd") or ""])
                    n_rows += 1
                time.sleep(PAUSE)
        print(f"{pilot} {name}: 역 {len(route)}, 좌표 {sum(1 for s in route if s['stinCd'] in coords)}, "
              f"시각표 행 {n_rows} ({time.time() - t0:.0f}초)", flush=True)
    with open(OUT_STATIONS, "w", encoding="utf-8", newline="") as f:
        w = csv.writer(f)
        w.writerow(["line", "opr", "ln_cd", "ln_name", "stin_cd", "order", "name", "lat", "lon"])
        w.writerows(stations_rows)
    with open(OUT_TIMETABLE, "w", encoding="utf-8", newline="") as f:
        w = csv.writer(f)
        w.writerow(["line", "day", "trn_no", "stin_cd", "arv", "dep", "org", "tmn"])
        w.writerows(tt_rows)
    print(f"역 {len(stations_rows)}행 → {OUT_STATIONS}\n시각표 {len(tt_rows)}행 → {OUT_TIMETABLE}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
