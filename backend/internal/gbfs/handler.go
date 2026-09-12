package gbfs

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Handler 는 /gbfs/*.json 을 GBFS 2.3 형식으로 낸다. OTP 는 gbfs.json 의 feeds 를 따라 나머지를 읽는다.
type Handler struct {
	Poller  *Poller
	BaseURL string // OTP 가 접근하는 이 서버의 주소, 예: http://localhost:8081/gbfs
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/gbfs/")
	snap, stale := h.Poller.Current()
	if snap == nil {
		http.Error(w, `{"error":"gbfs snapshot not ready"}`, http.StatusServiceUnavailable)
		return
	}
	ts := snap.FetchedAt.Unix()
	var data any
	switch name {
	case "gbfs.json":
		feeds := []map[string]string{}
		for _, f := range []string{"system_information", "station_information", "station_status"} {
			feeds = append(feeds, map[string]string{"name": f, "url": h.BaseURL + "/" + f + ".json"})
		}
		data = map[string]any{"ko": map[string]any{"feeds": feeds}}
	case "system_information.json":
		data = map[string]any{"system_id": SystemID, "language": "ko", "name": "서울 따릉이", "timezone": "Asia/Seoul"}
	case "station_information.json":
		list := make([]map[string]any, 0, len(snap.Stations))
		for _, s := range snap.Stations {
			list = append(list, map[string]any{"station_id": s.ID, "name": s.Name, "lat": s.Lat, "lon": s.Lon,
				"capacity": s.Capacity})
		}
		data = map[string]any{"stations": list}
	case "station_status.json":
		list := make([]map[string]any, 0, len(snap.Stations))
		for _, s := range snap.Stations {
			docks := s.Capacity - s.Bikes
			if docks < 0 {
				docks = 0 // 따릉이는 만차여도 반납이 되므로 OTP updater 의 overloadingAllowed 로 흡수한다
			}
			list = append(list, map[string]any{
				"station_id": s.ID, "num_bikes_available": s.Bikes, "num_docks_available": docks,
				"is_installed": true, "is_renting": !stale, "is_returning": true, "last_reported": ts,
			})
		}
		data = map[string]any{"stations": list}
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if stale {
		w.Header().Set("X-GBFS-Stale", "true")
	}
	json.NewEncoder(w).Encode(map[string]any{
		"last_updated": ts, "ttl": int(PollInterval.Seconds()), "version": "2.3", "data": data,
	})
}
