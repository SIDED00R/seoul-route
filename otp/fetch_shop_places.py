"""소상공인시장진흥공단 상가(상권)정보에서 서울 상가를 뽑아 장소 부분 일치 검색용 CSV 로 저장한다.

python otp/fetch_shop_places.py [내려받은.zip]
zip 을 주지 않으면 공공데이터포털에서 현재 판을 내려받는다(로그인 불필요, 전국 약 350MB). 분기마다 새 판이 나오고
파일 ID 가 바뀌므로 데이터 페이지에서 그때그때 찾는다.
"""

import csv
import io
import os
import re
import sys
import tempfile
import urllib.parse
import urllib.request
import zipfile

PAGE = "https://www.data.go.kr/data/15083033/fileData.do"
META_URL = "https://www.data.go.kr/tcs/dss/selectFileDataDownload.do"
FILE_URL = "https://www.data.go.kr/cmm/cmm/fileDownload.do?atchFileId={}&fileDetailSn={}"
DST = os.path.join(os.path.dirname(__file__), "data", "shop-places-seoul.csv")
UA = {"User-Agent": "Mozilla/5.0", "Referer": PAGE}

# 원본 열(0부터): 상가업소번호 0, 상호명 1, 지점명 2, 상권업종소분류명 8, 시도명 12, 지번주소 24, 도로명주소 31, 경도 37, 위도 38
COLS = {"id": 0, "name": 1, "branch": 2, "category": 8, "sido": 12, "jibun": 24, "road": 31, "lon": 37, "lat": 38}


def download(path: str) -> None:
    with urllib.request.urlopen(urllib.request.Request(PAGE, headers=UA), timeout=60) as r:
        page = r.read().decode("utf-8", "replace")
    m = re.search(r"fn_fileDataDown\('15083033',\s*'(uddi:[0-9a-f-]+)'", page)
    if not m:
        raise SystemExit(f"데이터 페이지에서 파일 정보를 찾지 못함: {PAGE}")
    form = urllib.parse.urlencode({"publicDataPk": "15083033", "publicDataDetailPk": m.group(1), "atchFileId": "",
                                   "fileDetailSn": "1", "publicDataTyCode": "PR0051"}).encode()
    req = urllib.request.Request(META_URL, data=form, headers={**UA, "X-Requested-With": "XMLHttpRequest"})
    with urllib.request.urlopen(req, timeout=60) as r:
        meta = r.read().decode("utf-8")
    fid = re.search(r'"atchFileId"\s*:\s*"(FILE_\d+)"', meta)
    sn = re.search(r'"fileDetailSn"\s*:\s*"?(\d+)', meta)
    if not fid or not sn:
        raise SystemExit("내려받기 정보 응답에 파일 ID 가 없음")
    print("내려받는 중:", fid.group(1))
    req = urllib.request.Request(FILE_URL.format(fid.group(1), sn.group(1)), headers=UA)
    with urllib.request.urlopen(req, timeout=600) as r, open(path, "wb") as f:
        while chunk := r.read(1 << 20):
            f.write(chunk)


def zip_name(info: zipfile.ZipInfo) -> str:
    # 파일 이름은 cp949 로 적혀 있어 zipfile 이 cp437 로 잘못 읽는다. 유니코드 경로 확장 필드가 있는 항목은 이미
    # 제대로 읽혀 cp437 로 되돌릴 수 없다.
    try:
        return info.filename.encode("cp437").decode("cp949")
    except (UnicodeEncodeError, UnicodeDecodeError):
        return info.filename


def convert(src: str) -> None:
    """zip 의 서울 CSV 를 DST.part 에 끝까지 쓴 뒤 DST 로 바꿔 넣는다. 도중에 실패하면 기존 DST 는 그대로다."""
    part = DST + ".part"
    try:
        with zipfile.ZipFile(src) as z:
            seoul = [i for i in z.infolist() if "_서울_" in zip_name(i) and zip_name(i).endswith(".csv")]
            if len(seoul) != 1:
                raise SystemExit(f"zip 안에 서울 CSV 가 하나가 아님: {[zip_name(i) for i in seoul]}")
            name = zip_name(seoul[0])
            kept = skipped = 0
            with z.open(seoul[0]) as f, open(part, "w", encoding="utf-8", newline="") as out:
                rows = csv.reader(io.TextIOWrapper(f, encoding="utf-8-sig"))
                head = next(rows)
                if head[COLS["name"]] != "상호명" or head[COLS["lat"]] != "위도":
                    raise SystemExit(f"열 구성이 바뀜: {head}")
                w = csv.writer(out)
                w.writerow(["id", "name", "branch", "category", "address", "lon", "lat"])
                for row in rows:
                    try:
                        lon, lat = float(row[COLS["lon"]]), float(row[COLS["lat"]])
                    except (ValueError, IndexError):
                        skipped += 1
                        continue
                    if row[COLS["sido"]] != "서울특별시" or not row[COLS["name"]].strip():
                        skipped += 1
                        continue
                    address = row[COLS["road"]].strip() or row[COLS["jibun"]].strip()
                    w.writerow([row[COLS["id"]], row[COLS["name"]].strip(), row[COLS["branch"]].strip(),
                                row[COLS["category"]].strip(), address, f"{lon:.7f}", f"{lat:.7f}"])
                    kept += 1
        if kept == 0:
            raise SystemExit(f"{name}: 서울 상가가 0곳(열 구성이 바뀌었을 수 있음)")
        # 쓰기 파일을 닫은 뒤에 바꾼다. Windows 는 열린 파일을 바꿔 넣지 못한다.
        os.replace(part, DST)
    except BaseException:
        if os.path.exists(part):
            os.remove(part)
        raise
    print(f"{name}: {kept}곳 → {DST} (좌표·이름 없음·서울 밖 {skipped}곳 제외)")


def main() -> int:
    src = sys.argv[1] if len(sys.argv) > 1 else None
    tmp = None
    try:
        if src is None:
            fd, tmp = tempfile.mkstemp(suffix=".zip")
            os.close(fd)
            download(tmp)
            src = tmp
        convert(src)
    finally:
        if tmp and os.path.exists(tmp):
            os.remove(tmp)
    return 0


if __name__ == "__main__":
    sys.exit(main())
