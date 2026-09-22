package httpapi

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

const favoritePlacesLimit = 20

func isFavoriteKindConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == "favorite_places_home_work_idx"
}

type favoritePlace struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Place Place  `json:"place"`
}

type favoritePlaceInput struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Place Place  `json:"place"`
}

func (in *favoritePlaceInput) normalize() error {
	in.Kind = strings.TrimSpace(in.Kind)
	in.Label = strings.TrimSpace(in.Label)
	in.Place.Name = strings.TrimSpace(in.Place.Name)
	in.Place.Address = strings.TrimSpace(in.Place.Address)
	in.Place.Category = strings.TrimSpace(in.Place.Category)
	if in.Kind != "home" && in.Kind != "work" && in.Kind != "custom" {
		return errors.New("kind 는 home·work·custom 중 하나")
	}
	if in.Label == "" || utf8.RuneCountInString(in.Label) > 20 {
		return errors.New("label 은 1~20자")
	}
	if in.Place.Name == "" || utf8.RuneCountInString(in.Place.Name) > 100 {
		return errors.New("장소 이름은 1~100자")
	}
	if math.IsNaN(in.Place.Lat) || math.IsNaN(in.Place.Lon) || math.IsInf(in.Place.Lat, 0) || math.IsInf(in.Place.Lon, 0) ||
		in.Place.Lat < route.MinLat || in.Place.Lat > route.MaxLat || in.Place.Lon < route.MinLon || in.Place.Lon > route.MaxLon {
		return errors.New("서울 범위의 장소 좌표가 필요하다")
	}
	return nil
}

func (s *Server) handleGetFavoritePlaces(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `
		SELECT id, kind, label, place_name, address, category, lat, lon
		FROM favorite_places WHERE user_id = $1
		ORDER BY CASE kind WHEN 'home' THEN 0 WHEN 'work' THEN 1 ELSE 2 END, created_at`, userIDFrom(r.Context()))
	if err != nil {
		s.Log.Error("favorite places query", "err", err)
		writeError(w, http.StatusInternalServerError, "즐겨찾기 조회 실패")
		return
	}
	defer rows.Close()
	out := []favoritePlace{}
	for rows.Next() {
		var f favoritePlace
		if err := rows.Scan(&f.ID, &f.Kind, &f.Label, &f.Place.Name, &f.Place.Address, &f.Place.Category,
			&f.Place.Lat, &f.Place.Lon); err != nil {
			writeError(w, http.StatusInternalServerError, "즐겨찾기 조회 실패")
			return
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "즐겨찾기 조회 실패")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"favorites": out})
}

func decodeFavoriteInput(w http.ResponseWriter, r *http.Request) (favoritePlaceInput, bool) {
	var in favoritePlaceInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "요청 본문 오류")
		return in, false
	}
	if err := in.normalize(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return in, false
	}
	return in, true
}

func (s *Server) handleCreateFavoritePlace(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeFavoriteInput(w, r)
	if !ok {
		return
	}
	userID := userIDFrom(r.Context())
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "즐겨찾기 추가 실패")
		return
	}
	defer tx.Rollback(r.Context())
	var activeUser int
	err = tx.QueryRow(r.Context(),
		`SELECT 1 FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&activeUser)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "사용자 없음")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "즐겨찾기 추가 실패")
		return
	}
	var n int
	if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM favorite_places WHERE user_id = $1`, userID).Scan(&n); err != nil {
		writeError(w, http.StatusInternalServerError, "즐겨찾기 추가 실패")
		return
	}
	if n >= favoritePlacesLimit {
		writeError(w, http.StatusConflict, "즐겨찾기는 20개까지")
		return
	}
	var f favoritePlace
	err = tx.QueryRow(r.Context(), `
		INSERT INTO favorite_places(user_id, kind, label, place_name, address, category, lat, lon)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, kind, label, place_name, address, category, lat, lon`,
		userID, in.Kind, in.Label, in.Place.Name, in.Place.Address, in.Place.Category, in.Place.Lat, in.Place.Lon).
		Scan(&f.ID, &f.Kind, &f.Label, &f.Place.Name, &f.Place.Address, &f.Place.Category, &f.Place.Lat, &f.Place.Lon)
	if err != nil {
		if isFavoriteKindConflict(err) {
			writeError(w, http.StatusConflict, "집·회사는 각각 하나만 저장할 수 있다")
			return
		}
		writeError(w, http.StatusInternalServerError, "즐겨찾기 추가 실패")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "즐겨찾기 추가 실패")
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handleUpdateFavoritePlace(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeFavoriteInput(w, r)
	if !ok {
		return
	}
	var f favoritePlace
	err := s.DB.QueryRow(r.Context(), `
		UPDATE favorite_places SET kind=$1,label=$2,place_name=$3,address=$4,category=$5,lat=$6,lon=$7,updated_at=now()
		WHERE id=$8 AND user_id=$9
		RETURNING id, kind, label, place_name, address, category, lat, lon`,
		in.Kind, in.Label, in.Place.Name, in.Place.Address, in.Place.Category, in.Place.Lat, in.Place.Lon,
		chi.URLParam(r, "id"), userIDFrom(r.Context())).
		Scan(&f.ID, &f.Kind, &f.Label, &f.Place.Name, &f.Place.Address, &f.Place.Category, &f.Place.Lat, &f.Place.Lon)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "즐겨찾기 없음")
		return
	}
	if err != nil {
		if isFavoriteKindConflict(err) {
			writeError(w, http.StatusConflict, "집·회사는 각각 하나만 저장할 수 있다")
			return
		}
		writeError(w, http.StatusInternalServerError, "즐겨찾기 수정 실패")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleDeleteFavoritePlace(w http.ResponseWriter, r *http.Request) {
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM favorite_places WHERE id=$1 AND user_id=$2`,
		chi.URLParam(r, "id"), userIDFrom(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "즐겨찾기 삭제 실패")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "즐겨찾기 없음")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
