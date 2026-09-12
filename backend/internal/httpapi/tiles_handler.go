package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// handleTile 은 VWorld WMTS Base 타일을 대신 받아 앱에 넘긴다. VWorld 키는 서버에만 둔다.
// 경로: /tiles/{z}/{x}/{y}.png (XYZ 순서). VWorld 는 {z}/{y}/{x} 순서라 여기서 바꿔 끼운다.
func (s *Server) handleTile(w http.ResponseWriter, r *http.Request) {
	if s.VWorldKey == "" {
		writeError(w, http.StatusServiceUnavailable, "지도 타일 미설정")
		return
	}
	z, errZ := strconv.Atoi(chi.URLParam(r, "z"))
	x, errX := strconv.Atoi(chi.URLParam(r, "x"))
	y, errY := strconv.Atoi(chi.URLParam(r, "y"))
	if errZ != nil || errX != nil || errY != nil || z < 6 || z > 19 || x < 0 || y < 0 {
		writeError(w, http.StatusBadRequest, "타일 좌표 오류")
		return
	}
	u := fmt.Sprintf("https://api.vworld.kr/req/wmts/1.0.0/%s/Base/%d/%d/%d.png", s.VWorldKey, z, y, x)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "요청 생성 실패")
		return
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		// url.Error 에 키가 든 URL 이 실리므로 오류 원문은 남기지 않는다.
		s.Log.Warn("vworld tile", "z", z, "x", x, "y", y, "err", "요청 실패")
		writeError(w, http.StatusBadGateway, "지도 타일 실패")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.Log.Warn("vworld tile", "z", z, "x", x, "y", y, "status", resp.StatusCode)
		writeError(w, http.StatusBadGateway, "지도 타일 실패")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, io.LimitReader(resp.Body, 2<<20))
}
