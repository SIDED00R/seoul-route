// Package auth 는 Google ID 토큰 검증과 서버 JWT 발급·검증을 맡는다.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenTTL 30일: 개인용 앱이라 재로그인 부담을 줄인다. 탈퇴 시 users.deleted_at 으로 무효화한다(미들웨어가 조회).
const TokenTTL = 30 * 24 * time.Hour

const issuer = "seoul-route"

type JWT struct {
	secret []byte
	now    func() time.Time
}

func NewJWT(secret string) *JWT {
	return &JWT{secret: []byte(secret), now: time.Now}
}

// Issue 는 user id 를 subject 로 하는 HS256 토큰을 만든다.
func (j *JWT) Issue(userID string) (string, error) {
	now := j.now()
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.secret)
}

// Verify 는 서명·만료·발급자를 검사하고 user id 를 돌려준다.
func (j *JWT) Verify(token string) (string, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("서명 방식 %v 거부", t.Header["alg"])
		}
		return j.secret, nil
	}, jwt.WithIssuer(issuer), jwt.WithTimeFunc(j.now), jwt.WithExpirationRequired())
	if err != nil {
		return "", err
	}
	if claims.Subject == "" {
		return "", errors.New("subject 없음")
	}
	return claims.Subject, nil
}
