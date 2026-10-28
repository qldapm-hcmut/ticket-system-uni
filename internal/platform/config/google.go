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
