package auth

import "strings"

// EmailAllowlist 는 Google 로그인을 허용할 계정 이메일 목록이다(AUTH_ALLOWED_EMAILS). 비어 있으면 모든 계정을 허용한다.
// 개발 서버에는 테스트 계정만, 운영 서버에는 실제 개인 계정만 넣어 두 환경의 데이터가 섞이지 않게 한다.
type EmailAllowlist struct{ emails map[string]struct{} }

// ParseEmailAllowlist 는 쉼표로 구분한 이메일 목록을 읽는다. 앞뒤 공백을 지우고 대소문자를 구분하지 않는다.
func ParseEmailAllowlist(s string) EmailAllowlist {
	a := EmailAllowlist{emails: map[string]struct{}{}}
	for _, e := range strings.Split(s, ",") {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" {
			a.emails[e] = struct{}{}
		}
	}
	return a
}

// Empty 는 목록이 비어 모든 계정을 허용하는 상태인지 알려준다.
func (a EmailAllowlist) Empty() bool { return len(a.emails) == 0 }

// Allows 는 이메일이 목록에 있는지 본다. 목록이 있으면 Google 이 소유를 확인한(email_verified) 이메일만 인정한다.
func (a EmailAllowlist) Allows(email string, verified bool) bool {
	if a.Empty() {
		return true
	}
	if !verified {
		return false
	}
	_, ok := a.emails[strings.ToLower(strings.TrimSpace(email))]
	return ok
}
