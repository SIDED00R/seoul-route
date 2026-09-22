package auth

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/api/idtoken"
)

// Identity 는 검증된 Google ID 토큰에서 읽은 계정 정보다. Sub 만 저장하고 Email 은 허용목록 대조에만 쓴다.
type Identity struct {
	Sub           string
	Email         string
	EmailVerified bool
}

// GoogleVerifier 는 앱이 보낸 Google ID 토큰을 검증해 안정적인 사용자 식별자(sub)와 이메일을 돌려준다.
type GoogleVerifier interface {
	Verify(ctx context.Context, idToken string) (Identity, error)
}

// googleVerifier 는 Google 공개키(JWKS)로 서명·만료·audience 를 검사한다.
type googleVerifier struct{ clientID string }

func NewGoogleVerifier(clientID string) (GoogleVerifier, error) {
	if clientID == "" {
		return nil, errors.New("GOOGLE_OAUTH_CLIENT_ID 없음")
	}
	return &googleVerifier{clientID: clientID}, nil
}

func (g *googleVerifier) Verify(ctx context.Context, idToken string) (Identity, error) {
	p, err := idtoken.Validate(ctx, idToken, g.clientID)
	if err != nil {
		return Identity{}, fmt.Errorf("google id token: %w", err)
	}
	if p.Subject == "" {
		return Identity{}, errors.New("google id token: sub 없음")
	}
	email, _ := p.Claims["email"].(string)
	verified, _ := p.Claims["email_verified"].(bool)
	return Identity{Sub: p.Subject, Email: email, EmailVerified: verified}, nil
}
