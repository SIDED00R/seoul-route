package ktdb

import (
	"os"
	"path/filepath"
	"testing"
)

// 파일럿 형식의 소형 데이터셋으로 Load 배선을 탄다. routes.txt 에는 BOM 을 붙여 실제 파일과 같게 한다.
func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"routes.txt": "\xef\xbb\xbfroute_id,agency_id,route_short_name,route_long_name,route_type\n" +
			"RR_1,A1,1호선,-,1\nBR_9,A1,9,-,0\nRR_KTX,A1,KTX,-,6\n",
		"stops.txt": "\xef\xbb\xbfstop_id,stop_name,stop_lat,stop_lon\n" +
			"RS_A,서울역,37.5547,126.9707\nRS_B,시청,37.5641,126.9770\n" +
			"RS_FAR,춘천,37.8850,127.7170\nBS_1,버스정류장,37.55,126.97\n",
		"trips.txt": "\xef\xbb\xbfroute_id,service_id,trip_id\n" +
			"RR_1,B1,RR_1_Ord001\nRR_1,B1,RR_1_Ord002\nBR_9,B1,BR_9_Ord001\nRR_KTX,B1,RR_KTX_Ord001\n",
		"stop_times.txt": "trip_id,arrival_time,departure_time,stop_id,stop_sequence,pickup_type,drop_off_type,timepoint\n" +
			"RR_1_Ord001,05:00:00,05:00:00,RS_A,1,0,0,1\nRR_1_Ord001,05:03:00,05:03:00,RS_B,2,0,0,1\n" +
			"RR_1_Ord002,06:00:00,06:00:00,RS_FAR,1,0,0,1\n" + // bbox 밖만 지나는 trip → 제외
			"BR_9_Ord001,05:00:00,05:00:00,BS_1,1,0,0,1\n" + // 버스 → 제외
			"RR_KTX_Ord001,05:00:00,05:00:00,RS_A,1,0,0,1\n", // 고속철도(6) → 제외
		"transfers.txt": "\xef\xbb\xbffrom_stop_id,to_stop_id,transfer_type,min_transfer_time\n" +
			"RS_A,RS_B,2,120\nRS_A,RS_FAR,2,120\n",
		"calendar.txt": "service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date\n" +
			"B1,1,1,1,1,1,1,1,20170101,20301231\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadKeepsOnlySubwayTripsThroughBBox(t *testing.T) {
	dir := writeFixture(t)
	s, err := Load(dir, BBox{MinLon: 126.70, MinLat: 37.38, MaxLon: 127.25, MaxLat: 37.75})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Trips) != 1 || s.Trips[0]["trip_id"] != "RR_1_Ord001" {
		t.Fatalf("trips=%v", s.Trips)
	}
	if len(s.Routes) != 1 || s.Routes[0]["route_id"] != "RR_1" {
		t.Fatalf("routes=%v (BOM 헤더가 route_id 로 읽혀야 한다)", s.Routes)
	}
	if len(s.StopTimes) != 2 || len(s.Stops) != 2 {
		t.Fatalf("stop_times=%d stops=%d", len(s.StopTimes), len(s.Stops))
	}
	if len(s.Transfers) != 1 || s.Transfers[0]["to_stop_id"] != "RS_B" {
		t.Fatalf("transfers=%v (양끝이 남은 역인 것만)", s.Transfers)
	}
}
