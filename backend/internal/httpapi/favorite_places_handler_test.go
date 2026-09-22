package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFavoritePlacesCRUD(t *testing.T) {
	s, pool := testServer(t)
	h := s.Router()
	sub := "sub-favorites-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = $1`, sub) })
	_, login := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, "")
	token := login["token"].(string)
	body := `{"kind":"home","label":"집","place":{"name":"우리집","address":"서울 양천구",` +
		`"category":"주거","lat":37.53,"lon":126.87}}`
	rr, created := do(t, h, http.MethodPost, "/users/me/favorites", body, token)
	if rr.Code != http.StatusCreated || created["id"] == nil {
		t.Fatalf("create code=%d body=%v", rr.Code, created)
	}
	id := created["id"].(string)
	if rr, _ := do(t, h, http.MethodPost, "/users/me/favorites", body, token); rr.Code != http.StatusConflict {
		t.Errorf("집 중복 code=%d", rr.Code)
	}

	rr, out := do(t, h, http.MethodGet, "/users/me/favorites", "", token)
	list := out["favorites"].([]any)
	if rr.Code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list code=%d body=%v", rr.Code, out)
	}
	updated := `{"kind":"custom","label":"부모님댁","place":{"name":"부모님댁","address":"서울 양천구",` +
		`"lat":37.531,"lon":126.871}}`
	if rr, out = do(t, h, http.MethodPut, "/users/me/favorites/"+id, updated, token); rr.Code != http.StatusOK || out["label"] != "부모님댁" {
		t.Fatalf("update code=%d body=%v", rr.Code, out)
	}
	if rr, _ = do(t, h, http.MethodDelete, "/users/me/favorites/"+id, "", token); rr.Code != http.StatusNoContent {
		t.Fatalf("delete code=%d", rr.Code)
	}
	_, out = do(t, h, http.MethodGet, "/users/me/favorites", "", token)
	if len(out["favorites"].([]any)) != 0 {
		t.Errorf("삭제 뒤=%v", out)
	}
}

func TestFavoritePlacesValidation(t *testing.T) {
	s, pool := testServer(t)
	h := s.Router()
	sub := "sub-favorites-invalid-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = $1`, sub) })
	_, login := do(t, h, http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, "")
	token := login["token"].(string)
	for _, body := range []string{
		`{"kind":"auto","label":"장소","place":{"name":"A","lat":37.5,"lon":127}}`,
		`{"kind":"custom","label":"","place":{"name":"A","lat":37.5,"lon":127}}`,
		`{"kind":"custom","label":"장소","place":{"name":"A","lat":35.1,"lon":129}}`,
	} {
		if rr, _ := do(t, h, http.MethodPost, "/users/me/favorites", body, token); rr.Code != http.StatusBadRequest {
			t.Errorf("body=%s code=%d", body, rr.Code)
		}
	}
}

func TestCreateFavoritePlaceDoesNotResurrectAfterAccountDeletion(t *testing.T) {
	s, pool := testServer(t)
	sub := "sub-favorites-deleted-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE google_sub = $1`, sub) })
	_, login := do(t, s.Router(), http.MethodPost, "/auth/google", `{"id_token":"good:`+sub+`"}`, "")
	if login["token"] == nil {
		t.Fatal("로그인 실패")
	}
	var userID string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM users WHERE google_sub = $1`, sub).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE users SET deleted_at = now() WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	body := `{"kind":"custom","label":"테스트","place":{"name":"테스트","lat":37.53,"lon":126.87}}`
	req := httptest.NewRequest(http.MethodPost, "/users/me/favorites", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxUserID, userID))
	rr := httptest.NewRecorder()
	s.handleCreateFavoritePlace(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM favorite_places WHERE user_id = $1`, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("탈퇴 뒤 즐겨찾기=%d", count)
	}
}
