// Package gtfszip 은 생성 GTFS zip 안의 CSV 를 읽는다.
package gtfszip

import (
	"archive/zip"
	"encoding/csv"
	"io"
	"strings"
)

// Reader 는 열어 둔 zip 하나다. 다 쓰면 Close 한다.
type Reader struct{ zr *zip.ReadCloser }

func Open(path string) (*Reader, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	return &Reader{zr}, nil
}

func (r *Reader) Close() error { return r.zr.Close() }

// Rows 는 CSV 한 장을 헤더 이름 → 값 맵의 목록으로 읽는다. 그 파일이 없으면 found 가 false 다.
func (r *Reader) Rows(name string) (rows []map[string]string, found bool, err error) {
	for _, f := range r.zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, true, err
		}
		defer rc.Close()
		cr := csv.NewReader(rc)
		header, err := cr.Read()
		if err != nil {
			return nil, true, err
		}
		header[0] = strings.TrimPrefix(header[0], "\xef\xbb\xbf")
		for {
			rec, err := cr.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, true, err
			}
			row := make(map[string]string, len(header))
			for i, h := range header {
				if i < len(rec) {
					row[h] = rec[i]
				}
			}
			rows = append(rows, row)
		}
		return rows, true, nil
	}
	return nil, false, nil
}
