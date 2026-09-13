"""공공데이터포털 "서울교통공사_서울 도시철도 열차운행시각표"(1~9호선, 요일별, 열차코드 포함) CSV 를 내려받아 UTF-8 로 저장한다.

사용: python otp/fetch_metro_timetable.py → otp/data/seoul-metro-timetable.csv
GTFS 생성기(gtfs/internal/seoulmetro)가 이 파일로 1~9호선 trip 을 만든다(파일 없으면 국가교통DB 파일럿 trip 그대로).
원본은 cp949, 열: 고유번호,호선,역사코드,역사명,주중주말(DAY/SAT/END),방향(UP/DOWN/IN/OUT),급행여부(0/1),열차코드,
열차도착시간,열차출발시간,출발역,도착역. 2026-09-13 실측 423,107행·열차 12,565편·역 코드 458(파일럿과 457개 일치).
데이터 페이지: https://www.data.go.kr/data/15098251/fileData.do (로그인 불필요). 파일 ID 는 페이지 갱신 시 바뀔 수 있다.
"""

import os
import sys
import urllib.request

PAGE = "https://www.data.go.kr/data/15098251/fileData.do"
FILE_URL = ("https://www.data.go.kr/cmm/cmm/fileDownload.do"
            "?atchFileId=FILE_000000007644502&fileDetailSn=1&insertDataPrcus=N")
DST = os.path.join(os.path.dirname(__file__), "data", "seoul-metro-timetable.csv")


def main() -> int:
    req = urllib.request.Request(FILE_URL, headers={"User-Agent": "Mozilla/5.0", "Referer": PAGE})
    with urllib.request.urlopen(req, timeout=300) as r:
        raw = r.read()
    if not raw.startswith(b'"') and b"," not in raw[:200]:
        print("CSV 가 아닌 응답(파일 ID 가 바뀌었을 수 있음). 페이지에서 atchFileId 를 다시 확인:", PAGE, file=sys.stderr)
        return 1
    text = raw.decode("cp949")
    with open(DST, "w", encoding="utf-8", newline="") as f:
        f.write(text)
    print(f"{text.count(chr(10))}행 → {DST}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
