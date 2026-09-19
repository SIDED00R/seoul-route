package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// VWorldBaseURL 은 VWorld API 주소. 테스트에서 Server.VWorldBase 로 바꿔 끼운다.
const VWorldBaseURL = "https://api.vworld.kr"

// tileAttempts: 상류 요청 상한(처음 1회 + 재시도 1회). 타일 GET 은 같은 요청을 다시 보내도 결과가 같다.
const tileAttempts = 2

// tileAttemptTimeout: 시도 하나에 주는 시간. 요청 전체 데드라인(server.go 의 DefaultTimeout 30초)을 그대로 쓰면
// 첫 시도가 그 데드라인으로 끝났을 때 남은 시간이 없어 재시도가 상류에 닿지 못한다. 두 번을 시도하고도 남게 잡는다.
// 테스트에서 줄여 끼운다.
var tileAttemptTimeout = 10 * time.Second

// tileBody 는 본문을 다 읽고 닫을 때 그 시도의 컨텍스트를 거둔다. 성공한 시도에서 곧바로 cancel 하면 호출자가
// io.Copy 로 읽는 도중에 끊겨 타일이 잘린다.
type tileBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b tileBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

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
	base := s.VWorldBase
	if base == "" {
		base = VWorldBaseURL
	}
	u := fmt.Sprintf("%s/req/wmts/1.0.0/%s/Base/%d/%d/%d.png", base, s.VWorldKey, z, y, x)
	resp, kind, attempts := s.fetchTile(r.Context(), u)
	if resp == nil {
		// url.Error 에 키가 든 URL 이 실리므로 오류 원문 대신 종류만 남긴다. 앱이 요청을 거둔 것(지도를 넘김)은 실패가 아니다.
		if kind != "canceled" {
			s.Log.Warn("vworld tile", "z", z, "x", x, "y", y, "err", kind, "attempts", attempts)
		}
		writeError(w, http.StatusBadGateway, "지도 타일 실패")
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, io.LimitReader(resp.Body, 2<<20))
}

// fetchTile 은 타일을 받아 200 응답과 실제 상류 요청 횟수를 돌려준다. 연결 오류·시간 초과·상류 5xx 면 한 번 더
// 보내고, 끝내 실패하면 nil 과 실패 종류("timeout"·"transport"·"canceled"·"status 404" …)를 돌려준다. 4xx 는 다시
// 보내지 않는다. 시도마다 tileAttemptTimeout 을 따로 주되, 요청 전체가 끝났으면(앱이 거뒀거나 라우트 데드라인)
// 다시 보내도 상류에 닿지 않으므로 그만둔다.
func (s *Server) fetchTile(ctx context.Context, u string) (*http.Response, string, int) {
	kind := ""
	attempts := 0
	for attempts < tileAttempts {
		attempts++
		try, cancel := context.WithTimeout(ctx, tileAttemptTimeout)
		req, err := http.NewRequestWithContext(try, http.MethodGet, u, nil)
		if err != nil {
			cancel()
			return nil, "request", attempts
		}
		resp, err := s.HTTP.Do(req)
		if err != nil {
			cancel()
			kind = tileErrKind(err)
			if ctx.Err() != nil {
				return nil, kind, attempts
			}
			continue
		}
		if resp.StatusCode == http.StatusOK {
			resp.Body = tileBody{resp.Body, cancel}
			return resp, "", attempts
		}
		resp.Body.Close()
		cancel()
		kind = "status " + strconv.Itoa(resp.StatusCode)
		if resp.StatusCode < 500 {
			return nil, kind, attempts
		}
	}
	return nil, kind, attempts
}

func tileErrKind(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	default:
		return "transport"
	}
}
