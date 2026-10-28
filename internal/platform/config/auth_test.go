package config

import (
	"strings"
	"testing"
)

func TestLoadAuthToken(t *testing.T) {
	for _, value := range []string{"", "short", strings.Repeat(" ", 40)} {
		t.Setenv("JWT_SECRET", value)
		if _, err := LoadAuthToken(); err == nil {
			t.Fatal("accepted invalid JWT_SECRET")
		}
	}
	secret := "test-only-signing-key-01234567890123456789"
	t.Setenv("JWT_SECRET", secret)
	c, err := LoadAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	if c.JWTSecret != secret {
		t.Fatal("secret unexpectedly changed")
	}
}
