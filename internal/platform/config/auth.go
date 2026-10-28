package config

import (
	"fmt"
	"os"
	"strings"
)

// AuthToken is loaded separately until authentication is wired into the server.
// Never log this value: it contains signing key material.
type AuthToken struct{ JWTSecret string }

func LoadAuthToken() (AuthToken, error) {
	secret := os.Getenv("JWT_SECRET")
	if len(strings.TrimSpace(secret)) < 32 {
		return AuthToken{}, fmt.Errorf("JWT_SECRET must contain at least 32 non-padding bytes")
	}
	return AuthToken{JWTSecret: secret}, nil
}
