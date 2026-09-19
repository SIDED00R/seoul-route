package seoulbus

import (
	"net/url"
	"sort"
)

// PathPoint 는 getRoutePath 의 한 행: 노선이 실제로 달리는 도로 위의 점. No 순서로 기점에서 종점까지 이어진다.
type PathPoint struct {
	No  string `json:"no"`
	Lon string `json:"gpsX"`
	Lat string `json:"gpsY"`
}

type pathResp struct {
	MsgHeader msgHeader `json:"msgHeader"`
	MsgBody   struct {
		ItemList []PathPoint `json:"itemList"`
	} `json:"msgBody"`
}

// RoutePath 는 노선 경로의 점들을 순서대로 돌려준다. 경로가 없는 노선은 빈 목록이다.
func (c *Client) RoutePath(routeID string) ([]PathPoint, error) {
	var resp pathResp
	if err := c.get("getRoutePath", url.Values{"busRouteId": {routeID}}, "path_"+routeID+".json", &resp); err != nil {
		return nil, err
	}
	pts := resp.MsgBody.ItemList
	sort.SliceStable(pts, func(i, j int) bool { return atoi(pts[i].No) < atoi(pts[j].No) })
	return pts, nil
}
