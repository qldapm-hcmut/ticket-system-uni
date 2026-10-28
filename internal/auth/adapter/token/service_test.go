package token

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-only-key-not-for-production-0123456789"

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func TestAccessToken(t *testing.T) {
	clock := &fakeClock{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	s, err := New(testSecret, clock)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.AccessToken("user-id", clock.Now().Add(15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.VerifyAccessToken(raw)
	if err != nil || id != "user-id" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	parsed, _, err := jwt.NewParser().ParseUnverified(raw, jwt.MapClaims{})
	if err != nil {
		t.Fatal(err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if len(claims) != 2 || claims["sub"] != "user-id" || claims["exp"] == nil {
		t.Fatalf("unexpected claims: %v", claims)
	}
	clock.now = clock.now.Add(15 * time.Minute)
	if _, err := s.VerifyAccessToken(raw); !errors.Is(err, ErrInvalidToken) {
		t.Fatal("accepted token at expiry")
	}
}

func TestRejectInvalidTokens(t *testing.T) {
	clock := &fakeClock{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	s, err := New(testSecret, clock)
	if err != nil {
		t.Fatal(err)
	}
	exp := jwt.NewNumericDate(clock.Now().Add(time.Hour))
	for _, tc := range []struct {
		name   string
		method jwt.SigningMethod
		claims jwt.RegisteredClaims
		key    any
	}{
		{"wrong key", jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "user", ExpiresAt: exp}, []byte(strings.Repeat("x", 32))},
		{"wrong algorithm", jwt.SigningMethodHS384, jwt.RegisteredClaims{Subject: "user", ExpiresAt: exp}, []byte(testSecret)},
		{"none", jwt.SigningMethodNone, jwt.RegisteredClaims{Subject: "user", ExpiresAt: exp}, jwt.UnsafeAllowNoneSignatureType},
		{"no expiry", jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "user"}, []byte(testSecret)},
		{"no subject", jwt.SigningMethodHS256, jwt.RegisteredClaims{ExpiresAt: exp}, []byte(testSecret)},
		{"future not before", jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "user", ExpiresAt: exp, NotBefore: jwt.NewNumericDate(clock.Now().Add(time.Minute))}, []byte(testSecret)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := jwt.NewWithClaims(tc.method, tc.claims).SignedString(tc.key)
			if err != nil {
				t.Fatal(err)
			}
			if id, err := s.VerifyAccessToken(raw); !errors.Is(err, ErrInvalidToken) || id != "" {
				t.Fatalf("accepted invalid token: %v", err)
			}
		})
	}
	for _, raw := range []string{"", "garbage", "a.b.c", strings.Repeat("x", 100)} {
		if _, err := s.VerifyAccessToken(raw); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestRefreshToken(t *testing.T) {
	s, err := New(testSecret, &fakeClock{time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		raw, err := s.NewRefreshToken()
		if err != nil {
			t.Fatal(err)
		}
		bytes, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(bytes) != 32 {
			t.Fatal("expected 256-bit token")
		}
		if seen[raw] {
			t.Fatal("duplicate token")
		}
		seen[raw] = true
		hash := s.HashRefreshToken(raw)
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != 32 || hash == raw || hash != s.HashRefreshToken(raw) {
			t.Fatal("invalid hash")
		}
	}
	if s.HashRefreshToken("a") == s.HashRefreshToken("b") {
		t.Fatal("hash mismatch")
	}
}

func TestInvalidConfigurationAndIssuance(t *testing.T) {
	clock := &fakeClock{time.Now()}
	for _, secret := range []string{"", "short", strings.Repeat(" ", 64)} {
		if _, err := New(secret, clock); err == nil {
			t.Fatal("accepted weak config")
		}
	}
	if _, err := New(testSecret, nil); err == nil {
		t.Fatal("accepted nil clock")
	}
	s, _ := New(testSecret, clock)
	for _, tc := range []struct {
		id      string
		expires time.Time
	}{{"", clock.Now().Add(time.Hour)}, {"user", clock.Now()}, {"user", clock.Now().Add(-time.Second)}} {
		if _, err := s.AccessToken(tc.id, tc.expires); err == nil {
			t.Fatal("accepted invalid issuance")
		}
	}
}
