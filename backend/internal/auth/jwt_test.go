package auth

import (
	"strings"
	"testing"
	"time"
)

func TestJWTIssueVerify(t *testing.T) {
	j := NewJWT(strings.Repeat("s", 32))
	tok, err := j.Issue("user-1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := j.Verify(tok)
	if err != nil || got != "user-1" {
		t.Fatalf("Verify=%q err=%v", got, err)
	}
}

func TestJWTRejectsWrongSecretAndExpiry(t *testing.T) {
	j := NewJWT(strings.Repeat("s", 32))
	tok, _ := j.Issue("user-1")
	if _, err := NewJWT(strings.Repeat("x", 32)).Verify(tok); err == nil {
		t.Fatal("다른 비밀키로 서명된 토큰이 통과했다")
	}
	later := NewJWT(strings.Repeat("s", 32))
	later.now = func() time.Time { return time.Now().Add(TokenTTL + time.Minute) }
	if _, err := later.Verify(tok); err == nil {
		t.Fatal("만료된 토큰이 통과했다")
	}
	if _, err := j.Verify("garbage"); err == nil {
		t.Fatal("형식이 아닌 토큰이 통과했다")
	}
}
