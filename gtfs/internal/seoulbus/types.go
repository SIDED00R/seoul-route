// Package seoulbus 는 서울시 버스 노선정보조회 API(ws.bus.go.kr/api/rest/busRouteInfo)의 클라이언트다.
package seoulbus

// Route 는 getBusRouteList 의 한 행이다.
type Route struct {
	ID        string `json:"busRouteId"`
	Name      string `json:"busRouteNm"`
	Abrv      string `json:"busRouteAbrv"`
	Type      string `json:"routeType"` // 1 공항 2 마을 3 간선 4 지선 5 순환 6 광역 7 인천 8 경기 (API 코드)
	StartName string `json:"stStationNm"`
	EndName   string `json:"edStationNm"`
	FirstBus  string `json:"firstBusTm"` // yyyyMMddHHmmss
	LastBus   string `json:"lastBusTm"`
	TermMin   string `json:"term"` // 배차간격(분)
	LengthKm  string `json:"length"`
	Corp      string `json:"corpNm"`
}

// Stop 은 getStaionByRoute 의 한 행이다.
type Stop struct {
	Seq       string `json:"seq"`
	StationID string `json:"station"` // 정류소 고유 ID
	ARSID     string `json:"arsId"`
	Name      string `json:"stationNm"`
	Lon       string `json:"gpsX"`
	Lat       string `json:"gpsY"`
	SectDist  string `json:"fullSectDist"` // 직전 정류장부터 거리(m). 첫 정류장은 0
	SectSpd   string `json:"sectSpd"`      // 구간 평균속도(km/h). 0 이면 결측
	BeginTm   string `json:"beginTm"`      // 정류장별 첫차 통과시각 HH:MM. 첫 정류장은 ":"
	LastTm    string `json:"lastTm"`
	Direction string `json:"direction"`
	TransYn   string `json:"transYn"` // Y = 회차 지점
}

type msgHeader struct {
	HeaderCd  string `json:"headerCd"`
	HeaderMsg string `json:"headerMsg"`
}

type routeListResp struct {
	MsgHeader msgHeader `json:"msgHeader"`
	MsgBody   struct {
		ItemList []Route `json:"itemList"`
	} `json:"msgBody"`
}

type stopListResp struct {
	MsgHeader msgHeader `json:"msgHeader"`
	MsgBody   struct {
		ItemList []Stop `json:"itemList"`
	} `json:"msgBody"`
}
