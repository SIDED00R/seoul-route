package auth

import "testing"

func TestEmailAllowlist(t *testing.T) {
	empty := ParseEmailAllowlist("")
	if !empty.Empty() || !empty.Allows("anyone@example.com", false) {
		t.Fatal("빈 목록은 모든 계정을 허용해야 한다")
	}
	if !ParseEmailAllowlist(" , ,").Empty() {
		t.Fatal("쉼표·공백만 있으면 빈 목록이다")
	}

	a := ParseEmailAllowlist(" Dev.Tester@Gmail.com , me@example.com")
	cases := []struct {
		email    string
		verified bool
		want     bool
	}{
		{"dev.tester@gmail.com", true, true},
		{"DEV.TESTER@gmail.com", true, true}, // 대소문자 무시
		{" me@example.com ", true, true},     // 앞뒤 공백 무시
		{"me@example.com", false, false},     // 목록에 있어도 미확인 이메일은 거부
		{"other@example.com", true, false},
		{"", true, false},
	}
	for _, c := range cases {
		if got := a.Allows(c.email, c.verified); got != c.want {
			t.Errorf("Allows(%q, %v) = %v, want %v", c.email, c.verified, got, c.want)
		}
	}
}
