package httpapi

import (
	"encoding/json"
	"net/http"
)

// handleAuthGoogle 은 앱의 Google ID 토큰을 검증하고, 처음 보는 sub 면 사용자를 만든 뒤 서버 JWT 를 돌려준다.
// 탈퇴한 사용자가 같은 Google 계정으로 다시 오면 새 행을 만들지 않고 401 을 준다(재가입은 별도 정책으로 미정).
func (s *Server) handleAuthGoogle(w http.ResponseWriter, r *http.Request) {
	if s.Google == nil {
		writeError(w, http.StatusServiceUnavailable, "Google 로그인 미설정")
		return
	}
	var in struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil || in.IDToken == "" {
		writeError(w, http.StatusBadRequest, "id_token 필요")
		return
	}
	sub, err := s.Google.Subject(r.Context(), in.IDToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Google 토큰 무효")
		return
	}
	var userID string
	var deleted bool
	err = s.DB.QueryRow(r.Context(), `
		INSERT INTO users (google_sub) VALUES ($1)
		ON CONFLICT (google_sub) DO UPDATE SET google_sub = EXCLUDED.google_sub
		RETURNING id, deleted_at IS NOT NULL`, sub).Scan(&userID, &deleted)
	if err != nil {
		s.Log.Error("auth upsert", "err", err)
		writeError(w, http.StatusInternalServerError, "저장 실패")
		return
	}
	if deleted {
		writeError(w, http.StatusUnauthorized, "탈퇴한 계정")
		return
	}
	token, err := s.JWT.Issue(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "토큰 발급 실패")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token, "user_id": userID})
}
