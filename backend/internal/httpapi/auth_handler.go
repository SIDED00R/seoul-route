package httpapi

import (
	"encoding/json"
	"net/http"
)

// handleAuthConfig 는 앱이 Google 로그인에 쓸 웹 클라이언트 ID 를 돌려준다(무인증). 클라이언트 ID 는 비밀이 아니라
// 공개 식별자다(앱 바이너리에 박아도 되는 값) — 서버가 내려주면 앱을 다시 빌드하지 않고 바꿀 수 있다. 미설정이면 빈 문자열.
func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"google_client_id": s.GoogleClientID})
}

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
	id, err := s.Google.Verify(r.Context(), in.IDToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Google 토큰 무효")
		return
	}
	// 환경별 허용 계정(AUTH_ALLOWED_EMAILS) 밖이면 사용자 행을 만들지 않고 403. 이메일은 로그에도 남기지 않는다.
	if !s.Allowed.Allows(id.Email, id.EmailVerified) {
		s.Log.Warn("google login denied", "reason", "허용목록 밖 계정")
		writeError(w, http.StatusForbidden, "이 서버에서 허용되지 않은 계정")
		return
	}
	sub := id.Sub
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
