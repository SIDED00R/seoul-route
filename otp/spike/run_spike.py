"""OTP 2.10 planConnection 스파이크. 로컬 OTP(8080)에 케이스별 질의를 보내고 leg 요약을 출력한다.

실행: python otp/spike/run_spike.py [케이스명 ...]   (인자 없으면 전부)
전제: OTP 서빙 중, GBFS fixture 서버(8090) 기동 중.
"""

import json
import os
import sys
import urllib.request

OTP = "http://localhost:8080/otp/gtfs/v1"
# 출발시각. 환경변수 SPIKE_DEPART=now 면 dateTime 을 생략해 "지금 출발"로 보낸다.
# OTP GTFS GraphQL API 는 지금 출발일 때만 대여소 실시간 잔여대수를 라우팅에 쓴다
# (RouteRequestMapper.setRentalAvailabilityPreferences: useAvailabilityInformation = isTripPlannedForNow).
DEPART = os.environ.get("SPIKE_DEPART", "2026-09-14T14:00:00+09:00")

SEOUL_STN = {"latitude": 37.5547, "longitude": 126.9707}
YONGSAN = {"latitude": 37.5299, "longitude": 126.9648}
YEOUIDO = {"latitude": 37.5216, "longitude": 126.9243}
GANGNAM = {"latitude": 37.4979, "longitude": 127.0276}
SINNONHYEON = {"latitude": 37.5045, "longitude": 127.0250}

QUERY = """
query Plan($origin: PlanLabeledLocationInput!, $destination: PlanLabeledLocationInput!,
           $via: [PlanViaLocationInput!], $modes: PlanModesInput, $preferences: PlanPreferencesInput,
           $dateTime: PlanDateTimeInput, $first: Int) {
  planConnection(origin: $origin, destination: $destination, via: $via, modes: $modes,
                 preferences: $preferences, dateTime: $dateTime, first: $first) {
    routingErrors { code description }
    edges { node {
      start end duration numberOfTransfers walkDistance generalizedCost
      legs { mode duration distance rentedBike transitLeg
             from { name } to { name } route { shortName } }
    } }
  }
}
"""


def loc(c, label=None):
    return {"label": label, "location": {"coordinate": c}}


def case(name, origin, dest, via=None, modes=None, prefs=None, first=5):
    return name, {
        "origin": loc(origin), "destination": loc(dest), "via": via, "modes": modes,
        "preferences": prefs, "first": first,
        "dateTime": None if DEPART == "now" else {"earliestDeparture": DEPART},
    }


def walk_speed(v):
    return {"street": {"walk": {"speed": v}}}


WALK_ONLY = {"directOnly": True, "direct": ["WALK"]}
# BICYCLE_RENTAL 은 같은 요청에 WALK 가 함께 있어야 한다(단독 지정은 OTP 2.10 BadRequestError 실측).
RENTAL_DIRECT = {"directOnly": True, "direct": ["BICYCLE_RENTAL", "WALK"]}
VIA_YEOUIDO = [{"visit": {"coordinate": YEOUIDO, "label": "여의도", "minimumWaitTime": "PT0S"}}]


def transit_modes(access, egress):
    return {"direct": ["WALK"], "transit": {"access": access, "egress": egress, "transfer": ["WALK"]}}


CASES = dict([
    case("walk_only", SEOUL_STN, YONGSAN, modes=WALK_ONLY),
    case("walk_speed_1.0", SEOUL_STN, YONGSAN, modes=WALK_ONLY, prefs=walk_speed(1.0)),
    case("walk_speed_1.6", SEOUL_STN, YONGSAN, modes=WALK_ONLY, prefs=walk_speed(1.6)),
    case("rental_direct", SEOUL_STN, YEOUIDO, modes=RENTAL_DIRECT),
    case("rental_direct_bike_speed_2.5", SEOUL_STN, YEOUIDO, modes=RENTAL_DIRECT,
         prefs={"street": {"bicycle": {"speed": 2.5}}}),
    case("rental_from_empty_station", SINNONHYEON, GANGNAM, modes=RENTAL_DIRECT),
    case("via_yeouido_direct_only", SEOUL_STN, GANGNAM, via=VIA_YEOUIDO, modes=RENTAL_DIRECT),
    # modes 를 생략하면 OTP 2.10 이 via 처리에서 NPE("modesArgs is null")를 낸다 — 항상 명시한다.
    case("via_yeouido_transit", SEOUL_STN, GANGNAM, via=VIA_YEOUIDO,
         modes=transit_modes(["BICYCLE_RENTAL", "WALK"], ["WALK"])),
    # 아래 둘은 GTFS 가 그래프에 들어간 뒤에만 transit leg 가 나온다.
    case("transit_rental_access", SEOUL_STN, GANGNAM, modes=transit_modes(["BICYCLE_RENTAL", "WALK"], ["WALK"])),
    case("transit_rental_egress", SEOUL_STN, GANGNAM, modes=transit_modes(["WALK"], ["BICYCLE_RENTAL", "WALK"])),
])


def run(name, variables):
    body = json.dumps({"query": QUERY, "variables": variables}).encode()
    req = urllib.request.Request(OTP, data=body, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=120) as resp:
        data = json.load(resp)
    print(f"\n### {name}")
    if "errors" in data:
        print("GraphQL errors:", json.dumps(data["errors"], ensure_ascii=False)[:800])
        return
    pc = data["data"]["planConnection"]
    if pc["routingErrors"]:
        print("routingErrors:", pc["routingErrors"])
    for i, e in enumerate(pc["edges"], 1):
        it = e["node"]
        legs = " → ".join(
            f"{l['mode']}{'(rent)' if l.get('rentedBike') else ''}"
            f"{'[' + (l['route'] or {}).get('shortName', '') + ']' if l.get('transitLeg') else ''}"
            f" {l['duration'] / 60:.1f}m/{l['distance'] / 1000:.2f}km"
            f"{' ' + l['from']['name'] + '→' + l['to']['name'] if l.get('rentedBike') else ''}"
            for l in it["legs"]
        )
        print(f"{i}. 총 {it['duration'] / 60:.1f}분  환승 {it['numberOfTransfers']}  "
              f"도보 {it['walkDistance']:.0f}m  cost {it['generalizedCost']}")
        print(f"   {legs}")


def main():
    names = sys.argv[1:] or list(CASES)
    for n in names:
        run(n, CASES[n])


if __name__ == "__main__":
    main()
