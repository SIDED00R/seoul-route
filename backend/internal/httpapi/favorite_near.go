package httpapi

import (
	"context"

	"github.com/SIDED00R/seoul-route/backend/internal/geo"
)

// favoriteNearRadiusM: 현재 위치를 즐겨찾기 장소로 부르는 거리 상한(m). 아파트 동 즐겨찾기(카카오 장소 좌표)와 그 동
// 안에서 받은 현재 위치 사이 거리 실측 20~41m(2026-09-28, 운영 6건). 이웃 건물도 이 안에 들 수 있다.
// 즐겨찾기 건물 안에서 다른 이름이 나오면 늘리고, 이웃 건물에서 즐겨찾기 이름이 나오면 줄인다.
const favoriteNearRadiusM = 50

// favoriteNear 는 사용자 즐겨찾기 중 favoriteNearRadiusM 안에서 가장 가까운 곳의 장소 이름을 돌려준다.
// 없거나 조회에 실패하면 빈 문자열(이름은 건물·주소로 대신한다).
func (s *Server) favoriteNear(ctx context.Context, userID string, lat, lon float64) string {
	rows, err := s.DB.Query(ctx, `SELECT place_name, lat, lon FROM favorite_places WHERE user_id = $1`, userID)
	if err != nil {
		s.Log.Error("favorite near query", "err", err)
		return ""
	}
	defer rows.Close()
	name, best := "", float64(favoriteNearRadiusM)
	for rows.Next() {
		var n string
		var flat, flon float64
		if err := rows.Scan(&n, &flat, &flon); err != nil {
			s.Log.Error("favorite near scan", "err", err)
			return ""
		}
		if d := geo.DistM(lat, lon, flat, flon); d <= best {
			name, best = n, d
		}
	}
	if err := rows.Err(); err != nil {
		s.Log.Error("favorite near rows", "err", err)
		return ""
	}
	return name
}
