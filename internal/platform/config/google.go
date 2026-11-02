package config

import (
	"errors"
	"os"
	"strings"
)

// LoadGoogleClientID is used when wiring the Google verifier, not by the worker.
func LoadGoogleClientID() (string, error) {
	id := os.Getenv("GOOGLE_CLIENT_ID")
	if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
		return "", errors.New("GOOGLE_CLIENT_ID is required and must not contain padding")
	}
	return id, nil
}

// LoadQRSecret returns the HMAC key for registration QR tokens. Length is
// validated by the token service; this only rejects a missing value.
func LoadQRSecret() (string, error) {
	secret := os.Getenv("QR_HMAC_SECRET")
	if strings.TrimSpace(secret) == "" {
		return "", errors.New("QR_HMAC_SECRET is required")
	}
	return secret, nil
}
