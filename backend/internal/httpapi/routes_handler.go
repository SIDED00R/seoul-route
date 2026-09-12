package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/SIDED00R/seoul-route/backend/internal/otp"
	"github.com/SIDED00R/seoul-route/backend/internal/route"
)

// handlePlan 은 인증된 사용자의 경로 요청을 받아 개인 속도(speed_profiles)를 주입하고 Planner 에 넘긴다.
func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	var req route.PlanRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "요청 본문 오류")
		return
	}
	userID := userIDFrom(r.Context())
	// 개인 속도는 보조 정보라 조회 실패 시 기본값으로 계속 진행하되, 실패를 로그에 남긴다(무음 금지).
	rows, err := s.DB.Query(r.Context(),
		`SELECT mode, speed_mps FROM speed_profiles WHERE user_id = $1 AND speed_mps IS NOT NULL`, userID)
	if err != nil {
		s.Log.Warn("speed profile query", "err", err)
	} else {
		for rows.Next() {
			var mode string
			var v float64
			if err := rows.Scan(&mode, &v); err != nil {
				s.Log.Warn("speed profile scan", "err", err)
				continue
			}
			switch mode {
			case "walk":
				req.WalkSpeed = v
			case "bicycle":
				req.BikeSpeed = v
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			s.Log.Warn("speed profile rows", "err", err)
		}
	}
	if req.WalkSpeed <= 0 {
		req.WalkSpeed = route.DefaultWalk
	}
	if req.BikeSpeed <= 0 {
		req.BikeSpeed = route.DefaultBike
	}
	its, err := s.Planner.Plan(r.Context(), req)
	switch {
	case errors.Is(err, route.ErrBadRequest):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, otp.ErrNoRoute):
		writeJSON(w, http.StatusOK, map[string]any{"itineraries": []any{}, "reason": err.Error()})
		return
	case err != nil:
		s.Log.Error("plan", "err", err)
		writeError(w, http.StatusBadGateway, "경로 엔진 오류")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"itineraries": its,
		"walk_speed":  req.WalkSpeed,
		"bike_speed":  req.BikeSpeed,
		"note": "경유지가 있거나 구간별 수단을 고정하면 구간 제약을 순차 적용한 후보가 섞이며 전역 최적이 아니다. " +
			"고정한 수단으로 경로가 없으면 빈 목록이다. 따릉이 잔여대수는 지금 출발일 때만 반영된다.",
	})
}
