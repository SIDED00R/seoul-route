package httpapi

import (
	"net/http"
	"time"
)

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r.Context())
	var createdAt time.Time
	err := s.DB.QueryRow(r.Context(), `SELECT created_at FROM users WHERE id = $1`, userID).Scan(&createdAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "사용자 없음")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "created_at": createdAt})
}

// handleDeleteMe 는 탈퇴: 사용자 행에 deleted_at 을 찍고 그 사용자의 trip·속도 프로파일·최근 경로·즐겨찾기를 즉시 지운다.
// 이후 같은 토큰은 requireAuth 에서 거부된다. 궤적(traces)은 trips 의 ON DELETE CASCADE 로 같이 지워진다.
func (s *Server) handleDeleteMe(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r.Context())
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "삭제 실패")
		return
	}
	defer tx.Rollback(r.Context())
	// 사용자 행을 먼저 잠근다. 같은 행을 잠그는 saveRecentRoute 와 순서가 정해져, 탈퇴 도중에 들어온 검색이
	// 지운 뒤에 기록을 되살리지 못한다(먼저 잠근 쪽이 끝난 뒤 상대가 판정을 다시 한다).
	for _, q := range []string{
		`UPDATE users SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`,
		`DELETE FROM recent_routes WHERE user_id = $1`,
		`DELETE FROM favorite_places WHERE user_id = $1`,
		`DELETE FROM speed_profiles WHERE user_id = $1`,
		`DELETE FROM trips WHERE user_id = $1`,
	} {
		if _, err := tx.Exec(r.Context(), q, userID); err != nil {
			s.Log.Error("delete user", "err", err)
			writeError(w, http.StatusInternalServerError, "삭제 실패")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "삭제 실패")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
