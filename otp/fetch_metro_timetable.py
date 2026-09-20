"""서울교통공사 열차운행시각표 CSV를 내려받아 UTF-8로 저장한다."""

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
