"""국가교통DB GTFS 파일럿(전국)에서 서울 bbox 를 지나는 trip 만 남겨 otp/data/gtfs-ktdb.zip 을 만든다.

규칙: bbox 안 정류장을 하나라도 지나는 trip 은 통째로 남긴다(경계 밖 구간도 유지). 그 trip 이 쓰는
route·stop 만 남기고 agency·calendar·transfers 는 참조가 살아 있는 행만 남긴다.
bbox 는 otp/extract_seoul.py 와 같다.
"""

import csv
import io
import os
import sys
import time
import zipfile

DATA = os.path.join(os.path.dirname(os.path.abspath(__file__)), "data")
SRC = os.path.join(DATA, "202503_GTFS_DataSet")
DST = os.path.join(DATA, "gtfs-ktdb.zip")
MIN_LON, MIN_LAT, MAX_LON, MAX_LAT = 126.70, 37.38, 127.25, 37.75

# KTDB 파일럿의 route_type 은 국제 표준과 다르다(202503_GTFS_설명서.pdf 4쪽: 0 시내/마을버스, 1 도시철도,
# 2 해운, 3 시외버스, 4 일반철도, 5 공항버스, 6 고속철도, 7 항공). GTFS 표준(3 버스, 1 지하철, 2 철도, 4 페리)과
# OTP 확장코드(1100 항공)로 바꾼다.
ROUTE_TYPE_MAP = {"0": "3", "1": "1", "2": "4", "3": "3", "4": "2", "5": "3", "6": "2", "7": "1100"}


def log(msg):
    print(f"[{time.strftime('%H:%M:%S')}] {msg}", flush=True)


def read_rows(name):
    with open(os.path.join(SRC, name), encoding="utf-8-sig", newline="") as f:
        yield from csv.DictReader(f)


def main():
    if not os.path.isdir(SRC):
        print(f"입력 없음: {SRC}", file=sys.stderr)
        return 1

    stops_all = {}
    bbox_stops = set()
    for r in read_rows("stops.txt"):
        stops_all[r["stop_id"]] = r
        if MIN_LON <= float(r["stop_lon"]) <= MAX_LON and MIN_LAT <= float(r["stop_lat"]) <= MAX_LAT:
            bbox_stops.add(r["stop_id"])
    log(f"stops 전체 {len(stops_all):,}, bbox 안 {len(bbox_stops):,}")

    keep_trips = set()
    n = 0
    for r in read_rows("stop_times.txt"):
        n += 1
        if r["stop_id"] in bbox_stops:
            keep_trips.add(r["trip_id"])
    log(f"stop_times {n:,}행 1차 스캔, bbox 를 지나는 trip {len(keep_trips):,}")

    trips = [r for r in read_rows("trips.txt") if r["trip_id"] in keep_trips]
    keep_routes = {r["route_id"] for r in trips}
    keep_services = {r["service_id"] for r in trips}
    routes = [r for r in read_rows("routes.txt") if r["route_id"] in keep_routes]
    keep_agencies = {r["agency_id"] for r in routes}
    type_counts = {}
    for r in routes:
        src = r["route_type"]
        r["route_type"] = ROUTE_TYPE_MAP[src]
        type_counts[src] = type_counts.get(src, 0) + 1
    log(f"trips {len(trips):,}, routes {len(routes):,}, agencies {len(keep_agencies)}, "
        f"route_type(원본) 분포 {dict(sorted(type_counts.items()))}")

    keep_stops = set()
    stop_times_buf = io.StringIO()
    w = None
    m = 0
    for r in read_rows("stop_times.txt"):
        if r["trip_id"] in keep_trips:
            if w is None:
                w = csv.DictWriter(stop_times_buf, fieldnames=list(r.keys()), lineterminator="\n")
                w.writeheader()
            w.writerow(r)
            keep_stops.add(r["stop_id"])
            m += 1
    log(f"stop_times 2차 스캔: 남긴 행 {m:,}, 참조 stop {len(keep_stops):,}")

    def dump(rows, fieldnames):
        buf = io.StringIO()
        dw = csv.DictWriter(buf, fieldnames=fieldnames, lineterminator="\n")
        dw.writeheader()
        dw.writerows(rows)
        return buf.getvalue()

    stops = [stops_all[s] for s in keep_stops if s in stops_all]
    agency = [r for r in read_rows("agency.txt") if r["agency_id"] in keep_agencies]
    calendar = [r for r in read_rows("calendar.txt") if r["service_id"] in keep_services]
    transfers = [r for r in read_rows("transfers.txt")
                 if r["from_stop_id"] in keep_stops and r["to_stop_id"] in keep_stops]

    if os.path.exists(DST):
        os.remove(DST)
    with zipfile.ZipFile(DST, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("agency.txt", dump(agency, list(agency[0].keys())))
        z.writestr("calendar.txt", dump(calendar, list(calendar[0].keys())))
        z.writestr("routes.txt", dump(routes, list(routes[0].keys())))
        z.writestr("trips.txt", dump(trips, list(trips[0].keys())))
        z.writestr("stops.txt", dump(stops, list(stops[0].keys())))
        z.writestr("stop_times.txt", stop_times_buf.getvalue())
        z.writestr("transfers.txt",
                   dump(transfers, ["from_stop_id", "to_stop_id", "transfer_type", "min_transfer_time"]))
    log(f"완료 {DST} ({os.path.getsize(DST) / 1e6:.1f} MB): stops {len(stops):,} routes {len(routes):,} "
        f"trips {len(trips):,} stop_times {m:,} transfers {len(transfers):,}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
