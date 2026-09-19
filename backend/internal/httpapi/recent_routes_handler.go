package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

// RecentRoutesKept: 사용자마다 남기는 최근 경로 수. 홈 목록에 한 화면 넘게 쌓이지 않을 만큼만 둔다.
const RecentRoutesKept = 20

// recentRoute 는 목록 한 줄. Request 는 저장해 둔 요청 그대로라 앱이 그대로 다시 보내면 같은 검색이 된다.
type recentRoute struct {
	Request    json.RawMessage `json:"request"`
	SearchedAt string          `json:"searched_at"`
}

// handleGetRecentRoutes 는 최근 검색을 새 것부터 돌려준다.
func (s *Server) handleGetRecentRoutes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(),
		`SELECT request, searched_at FROM recent_routes WHERE user_id = $1 ORDER BY searched_at DESC LIMIT $2`,
		userIDFrom(r.Context()), RecentRoutesKept)
	if err != nil {
		s.Log.Error("recent routes query", "err", err)
		writeError(w, http.StatusInternalServerError, "최근 경로 조회 실패")
		return
	}
	defer rows.Close()
	out := []recentRoute{}
	for rows.Next() {
		var req []byte
		var at time.Time
		if err := rows.Scan(&req, &at); err != nil {
			s.Log.Error("recent routes scan", "err", err)
			writeError(w, http.StatusInternalServerError, "최근 경로 조회 실패")
			return
		}
		out = append(out, recentRoute{Request: req, SearchedAt: at.Format(time.RFC3339)})
	}
	if err := rows.Err(); err != nil {
		s.Log.Error("recent routes rows", "err", err)
		writeError(w, http.StatusInternalServerError, "최근 경로 조회 실패")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"routes": out})
}

// handleDeleteRecentRoutes 는 최근 경로를 전부 지운다. 어디를 다녔는지 남기고 싶지 않을 때 쓴다.
func (s *Server) handleDeleteRecentRoutes(w http.ResponseWriter, r *http.Request) {
	if _, err := s.DB.Exec(r.Context(), `DELETE FROM recent_routes WHERE user_id = $1`, userIDFrom(r.Context())); err != nil {
		s.Log.Error("recent routes delete", "err", err)
		writeError(w, http.StatusInternalServerError, "최근 경로 삭제 실패")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// saveRecentRoute 는 성공한 검색을 기록한다. 같은 요청이면 시각만 갱신하고, 오래된 것은 지운다.
// 경로 응답과는 무관한 부가 기능이라 실패해도 요청은 성공으로 둔다(로그만 남긴다).
func (s *Server) saveRecentRoute(ctx context.Context, userID string, req route.PlanRequest) {
	body, err := json.Marshal(map[string]any{
		"origin": req.Origin, "destination": req.Destination, "via": req.Via, "segment_modes": req.Modes,
	})
	if err != nil {
		s.Log.Warn("recent route marshal", "err", err)
		return
	}
	if _, err := s.DB.Exec(ctx, `
		INSERT INTO recent_routes(user_id, dedup_key, request, searched_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (user_id, dedup_key) DO UPDATE SET request = EXCLUDED.request, searched_at = now()`,
		userID, recentRouteKey(req), body); err != nil {
		s.Log.Warn("recent route save", "err", err)
		return
	}
	// 오래된 줄 정리. 지우는 데 실패해도 목록은 LIMIT 로 잘려 나가므로 로그만 남긴다.
	if _, err := s.DB.Exec(ctx, `
		DELETE FROM recent_routes
		WHERE user_id = $1 AND dedup_key NOT IN (
			SELECT dedup_key FROM recent_routes WHERE user_id = $1 ORDER BY searched_at DESC LIMIT $2)`,
		userID, RecentRoutesKept); err != nil {
		s.Log.Warn("recent route trim", "err", err)
	}
}

// recentRouteKey 는 같은 검색을 한 줄로 묶는 열쇠. 좌표는 소수 5자리(약 1m)로 줄여 손끝 차이로 줄이 늘지 않게 한다.
// 장소 이름은 넣지 않는다 — 같은 자리를 "현재 위치 · ○○빌딩" 과 검색 결과로 각각 고른 것을 따로 세지 않는다.
func recentRouteKey(req route.PlanRequest) string {
	var b strings.Builder
	point := func(p route.Point) { fmt.Fprintf(&b, "%.5f,%.5f>", p.Lat, p.Lon) }
	point(req.Origin)
	for _, v := range req.Via {
		point(v)
	}
	point(req.Destination)
	for _, m := range req.Modes {
		b.WriteString(string(m))
		b.WriteByte(',')
	}
	return b.String()
}
