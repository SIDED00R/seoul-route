package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// otpAlive 는 200 이어도 GraphQL 정상 본문이 아니면 실패해야 한다.
func TestOTPAliveChecksBody(t *testing.T) {
	cases := map[string]struct {
		body string
		ok   bool
	}{
		"정상":           {`{"data":{"__typename":"QueryType"}}`, true},
		"GraphQL 오류":   {`{"errors":[{"message":"Validation error"}]}`, false},
		"HTML 200 페이지": {`<html>oops</html>`, false},
		"빈 200":        {``, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(c.body))
			}))
			defer srv.Close()
			s := &Server{OTPURL: srv.URL, HTTP: srv.Client()}
			err := s.otpAlive(context.Background())
			if (err == nil) != c.ok {
				t.Fatalf("body=%q err=%v want ok=%v", c.body, err, c.ok)
			}
		})
	}
}
