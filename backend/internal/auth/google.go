package auth

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/api/idtoken"
)

// GoogleVerifier 는 앱이 보낸 Google ID 토큰을 검증해 안정적인 사용자 식별자(sub)를 돌려준다.
type GoogleVerifier interface {
	Subject(ctx context.Context, idToken string) (string, error)
}

// googleVerifier 는 Google 공개키(JWKS)로 서명·만료·audience 를 검사한다.
type googleVerifier struct{ clientID string }

func NewGoogleVerifier(clientID string) (GoogleVerifier, error) {
	if clientID == "" {
		return nil, errors.New("GOOGLE_OAUTH_CLIENT_ID 없음")
	}
	return &googleVerifier{clientID: clientID}, nil
}

func (g *googleVerifier) Subject(ctx context.Context, idToken string) (string, error) {
	p, err := idtoken.Validate(ctx, idToken, g.clientID)
	if err != nil {
		return "", fmt.Errorf("google id token: %w", err)
	}
	if p.Subject == "" {
		return "", errors.New("google id token: sub 없음")
	}
	return p.Subject, nil
}
