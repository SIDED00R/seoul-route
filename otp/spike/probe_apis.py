"""외부 API 표본 실측 (Phase 0). .env 의 키로 각 API 를 1~3회 호출해 응답 형식·건수를 요약한다.

실행: python otp/spike/probe_apis.py
출력: 콘솔 요약 + otp/spike/samples/*.json (응답 앞부분, 키는 URL 에만 있고 본문에는 없다)
키·URL 은 절대 출력하지 않는다.
"""

import json
import os
import sys
import urllib.parse
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
SAMPLES = os.path.join(ROOT, "otp", "spike", "samples")


def load_env():
    env = {}
    with open(os.path.join(ROOT, ".env"), encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                k, v = line.split("=", 1)
                env[k] = v.strip()
    return env


def get_json(url, label):
    try:
        with urllib.request.urlopen(url, timeout=60) as r:
            raw = r.read()
    except urllib.error.HTTPError as e:
        print(f"  [{label}] HTTP {e.code}: {e.read()[:200]!r}")
        return None
    try:
        data = json.loads(raw)
    except ValueError:
        print(f"  [{label}] JSON 아님, 앞 300자: {raw[:300]!r}")
        return None
    os.makedirs(SAMPLES, exist_ok=True)
    with open(os.path.join(SAMPLES, f"{label}.json"), "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=1)
    return data


# ws.bus.go.kr / openapi.seoul.go.kr 는 443 미개방(실측 2026-09-12) → http 유지.
def probe_bikelist(key):
    print("## 따릉이 bikeList (열린데이터광장)")
    d = get_json(f"http://openapi.seoul.go.kr:8088/{key}/json/bikeList/1/5/", "bikeList_p1")
    if not d:
        return
    body = d.get("rentBikeStatus") or d
    total = body.get("list_total_count")
    rows = body.get("row", [])
    print(f"  list_total_count={total}  1페이지 row={len(rows)}  RESULT={body.get('RESULT')}")
    if rows:
        print(f"  필드: {sorted(rows[0].keys())}")
        print(f"  예시: {rows[0]}")
    # list_total_count 는 전체가 아니라 요청 범위의 건수를 돌려준다(실측: 1/5 요청 → 5).
    # 전체 건수는 1,000행씩 넘기며 짧은 페이지가 나올 때까지 세야 한다.
    total_rows, ids = 0, set()
    for p in range(1, 10):
        d2 = get_json(f"http://openapi.seoul.go.kr:8088/{key}/json/bikeList/{(p - 1) * 1000 + 1}/{p * 1000}/",
                      f"bikeList_p{p}")
        if not d2:
            break
        rows = (d2.get("rentBikeStatus") or d2).get("row", [])
        total_rows += len(rows)
        ids.update(r["stationId"] for r in rows)
        if len(rows) < 1000:
            break
    print(f"  → 전체 row={total_rows} 고유 stationId={len(ids)} ({p}콜)")


def probe_bus(key):
    print("## 서울 버스 노선정보 (공공데이터포털, ws.bus.go.kr)")
    q = urllib.parse.urlencode({"serviceKey": key, "strSrch": "271", "resultType": "json"})
    d = get_json(f"http://ws.bus.go.kr/api/rest/busRouteInfo/getBusRouteList?{q}", "bus_getBusRouteList")
    if not d:
        return
    hdr = d.get("msgHeader", {})
    items = d.get("msgBody", {}).get("itemList") or []
    print(f"  getBusRouteList(strSrch=271): headerCd={hdr.get('headerCd')} {hdr.get('headerMsg')}  건수={len(items)}")
    if not items:
        return
    print(f"  필드: {sorted(items[0].keys())}")
    print(f"  예시: {items[0]}")
    rid = items[0]["busRouteId"]
    q = urllib.parse.urlencode({"serviceKey": key, "busRouteId": rid, "resultType": "json"})
    d = get_json(f"http://ws.bus.go.kr/api/rest/busRouteInfo/getRouteInfo?{q}", "bus_getRouteInfo")
    if d:
        it = (d.get("msgBody", {}).get("itemList") or [{}])[0]
        print(f"  getRouteInfo: 첫차={it.get('firstBusTm')} 막차={it.get('lastBusTm')} 배차={it.get('term')}분 "
              f"저상첫차={it.get('firstLowTm')} 노선길이={it.get('length')}km")
    d = get_json(f"http://ws.bus.go.kr/api/rest/busRouteInfo/getStaionByRoute?{q}", "bus_getStaionByRoute")
    if d:
        stops = d.get("msgBody", {}).get("itemList") or []
        print(f"  getStaionByRoute: 정류장 {len(stops)}개  필드: {sorted(stops[0].keys()) if stops else '-'}")
        if stops:
            s = stops[0]
            print(f"  예시: seq={s.get('seq')} {s.get('stationNm')} gps=({s.get('gpsY')},{s.get('gpsX')}) "
                  f"sectSpd={s.get('sectSpd')} fullSectDist={s.get('fullSectDist')} trnstnid={s.get('trnstnid')}")
    q = urllib.parse.urlencode({"serviceKey": key, "busRouteId": rid, "resultType": "json"})
    d = get_json(f"http://ws.bus.go.kr/api/rest/busRouteInfo/getRoutePath?{q}", "bus_getRoutePath")
    if d:
        pts = d.get("msgBody", {}).get("itemList") or []
        print(f"  getRoutePath: 좌표점 {len(pts)}개 (shapes.txt 용)")


def probe_subway(key):
    print("## TAGO 지하철정보 (공공데이터포털)")
    # 2022-09 TAGO 개편 후 경로는 /1613000/SubwayInfo, 오퍼레이션명은 대문자 Get… 이다.
    # 구 경로(SubwayInfoService/getKwrd…)는 NO_OPENAPI_SERVICE_ERROR(12) 를 낸다(실측 2026-09-12).
    base = "https://apis.data.go.kr/1613000/SubwayInfo"
    q = urllib.parse.urlencode({"serviceKey": key, "subwayStationName": "서울", "_type": "json",
                                "numOfRows": 20, "pageNo": 1})
    d = get_json(f"{base}/GetKwrdFndSubwaySttnList?{q}", "subway_GetKwrdFndSubwaySttnList")
    if not d:
        return
    resp = d.get("response", {})
    hdr = resp.get("header", {})
    items = resp.get("body", {}).get("items", {}).get("item") or []
    if isinstance(items, dict):
        items = [items]
    print(f"  역 검색(서울): resultCode={hdr.get('resultCode')} {hdr.get('resultMsg')}  "
          f"totalCount={resp.get('body', {}).get('totalCount')} 건수={len(items)}")
    if not items:
        return
    print(f"  필드: {sorted(items[0].keys())}  ← 좌표 없음: 역 좌표는 별도 소스 필요")
    for it in items[:5]:
        print(f"   - {it}")
    sid = items[0]["subwayStationId"]
    q = urllib.parse.urlencode({"serviceKey": key, "subwayStationId": sid, "dailyTypeCode": "01",
                                "upDownTypeCode": "U", "_type": "json", "numOfRows": 500, "pageNo": 1})
    d = get_json(f"{base}/GetSubwaySttnAcctoSchdulList?{q}", "subway_GetSubwaySttnAcctoSchdulList")
    if d:
        resp = d.get("response", {})
        body = resp.get("body", {})
        items = body.get("items", {}).get("item") or []
        if isinstance(items, dict):
            items = [items]
        print(f"  역별 시간표({sid}, 평일, 상행): totalCount={body.get('totalCount')} 건수={len(items)}")
        if items:
            print(f"  필드: {sorted(items[0].keys())}")
            print(f"  예시: {items[0]}")


def main():
    env = load_env()
    seoul = env.get("SEOUL_OPENAPI_KEY", "")
    gokr = env.get("DATA_GO_KR_KEY", "")
    if "%" in gokr:  # 공공데이터포털 Encoding 키면 디코딩해서 urlencode 에 맡긴다
        gokr = urllib.parse.unquote(gokr)
    if not seoul or not gokr:
        print("키 없음: SEOUL_OPENAPI_KEY / DATA_GO_KR_KEY 를 .env 에 넣어라", file=sys.stderr)
        return 1
    probe_bikelist(seoul)
    print()
    probe_bus(gokr)
    print()
    probe_subway(gokr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
