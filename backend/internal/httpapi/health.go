package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// handleHealth 는 DB ping 과 OTP GraphQL 응답을 실제로 확인한다. 하나라도 실패하면 503.
// 헬스가 200 이어도 기능 경로 검증을 대신하지 않는다(운영 스모크는 /routes/plan 실호출).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	status := map[string]string{"db": "ok", "otp": "ok"}
	code := http.StatusOK
	if err := s.DB.Ping(ctx); err != nil {
		status["db"] = "fail: " + err.Error()
		code = http.StatusServiceUnavailable
	}
	if err := s.otpAlive(ctx); err != nil {
		status["otp"] = "fail: " + err.Error()
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, status)
}

// otpAlive 는 `{ __typename }` 질의를 보내 OTP GraphQL 이 살아 있는지 본다. 상태코드만 보지 않고 본문의
// data.__typename 이 있고 errors 가 없는지까지 확인한다(OTP_URL 이 엉뚱한 200 서비스를 가리키는 오설정 탐지).
// 그래프가 비어 있는(|Stops|=0) OTP 도 이 질의에는 정상 응답하므로 기능 검증은 아니다.
func (s *Server) otpAlive(ctx context.Context) error {
	body := strings.NewReader(`{"query":"{ __typename }"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.OTPURL+"/otp/gtfs/v1", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &httpStatusError{resp.StatusCode}
	}
	var out struct {
		Data struct {
			Typename string `json:"__typename"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return errors.New("GraphQL 응답 아님")
	}
	if len(out.Errors) > 0 || out.Data.Typename == "" {
		return errors.New("GraphQL 오류 응답")
	}
	return nil
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return "HTTP " + http.StatusText(e.code) }
