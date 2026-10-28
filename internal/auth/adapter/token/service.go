// Package token implements local authentication token cryptography.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/shared/kernel"
)

var ErrInvalidToken = errors.New("invalid access token")

type Service struct {
	secret []byte
	clock  kernel.Clock
}

var _ application.TokenService = (*Service)(nil)

// New requires a high-entropy secret of at least 32 bytes, not a password.
func New(secret string, clock kernel.Clock) (*Service, error) {
	if len(strings.TrimSpace(secret)) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 non-padding bytes")
	}
	if clock == nil {
		return nil, errors.New("token clock is required")
	}
	return &Service{secret: []byte(secret), clock: clock}, nil
}

func (s *Service) NewRefreshToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (s *Service) HashRefreshToken(raw string) string {
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}

func (s *Service) AccessToken(userID string, expiresAt time.Time) (string, error) {
	if strings.TrimSpace(userID) == "" || !expiresAt.After(s.clock.Now()) {
		return "", errors.New("access token requires subject and future expiry")
	}
	// Roles and personal information intentionally do not belong in the JWT.
	claims := jwt.RegisteredClaims{Subject: userID, ExpiresAt: jwt.NewNumericDate(expiresAt)}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// VerifyAccessToken verifies only internal access tokens, never Google ID tokens.
// Authentication middleware must subsequently load the active user and current permissions.
func (s *Service) VerifyAccessToken(raw string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired(), jwt.WithTimeFunc(s.clock.Now))
	if err != nil || !parsed.Valid || strings.TrimSpace(claims.Subject) == "" {
		return "", ErrInvalidToken
	}
	return claims.Subject, nil
}
