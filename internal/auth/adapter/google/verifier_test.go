package google

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

// Production issuer remains pinned to Google. Only the key-fetch HTTP transport
// is redirected to a local server, so tests exercise real RSA/JWKS verification.
func TestVerifyGoogleIdentity(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/keys" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	defer server.Close()
	ctx := context.Background()
	keys := oidc.NewRemoteKeySet(ctx, server.URL+"/keys")
	v := &Verifier{verifier: oidc.NewVerifier("https://accounts.google.com", keys, &oidc.Config{ClientID: "test-client", SupportedSigningAlgs: []string{oidc.RS256}})}
	valid := func() jwt.MapClaims {
		return jwt.MapClaims{
			"iss": "https://accounts.google.com", "aud": "test-client", "sub": "google-sub",
			"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
			"email": "student@hcmut.edu.vn", "email_verified": true, "hd": "hcmut.edu.vn", "name": "Student",
		}
	}
	for _, tc := range []struct {
		name    string
		change  func(jwt.MapClaims)
		signer  *rsa.PrivateKey
		wantErr bool
	}{
		{name: "valid"},
		{name: "legacy Google issuer", change: func(c jwt.MapClaims) { c["iss"] = "accounts.google.com" }},
		{name: "wrong issuer", change: func(c jwt.MapClaims) { c["iss"] = "https://attacker.example" }, wantErr: true},
		{name: "wrong audience", change: func(c jwt.MapClaims) { c["aud"] = "other-client" }, wantErr: true},
		{name: "expired", change: func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() }, wantErr: true},
		{name: "missing expiry", change: func(c jwt.MapClaims) { delete(c, "exp") }, wantErr: true},
		{name: "wrong signature", signer: other, wantErr: true},
		{name: "unverified email", change: func(c jwt.MapClaims) { c["email_verified"] = false }, wantErr: true},
		{name: "missing verified", change: func(c jwt.MapClaims) { delete(c, "email_verified") }, wantErr: true},
		{name: "wrong hd", change: func(c jwt.MapClaims) { c["hd"] = "other.edu.vn" }, wantErr: true},
		{name: "missing hd", change: func(c jwt.MapClaims) { delete(c, "hd") }, wantErr: true},
		{name: "subdomain", change: func(c jwt.MapClaims) { c["email"] = "student@sub.hcmut.edu.vn" }, wantErr: true},
		{name: "suffix attack", change: func(c jwt.MapClaims) { c["email"] = "student@hcmut.edu.vn.evil.com" }, wantErr: true},
		{name: "display name", change: func(c jwt.MapClaims) { c["email"] = "Student <student@hcmut.edu.vn>" }, wantErr: true},
		{name: "missing sub", change: func(c jwt.MapClaims) { delete(c, "sub") }, wantErr: true},
		{name: "wrong claim type", change: func(c jwt.MapClaims) { c["email_verified"] = "true" }, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := valid()
			if tc.change != nil {
				tc.change(claims)
			}
			signer := key
			if tc.signer != nil {
				signer = tc.signer
			}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = "test-key"
			raw, err := token.SignedString(signer)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := v.Verify(ctx, raw)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidIdentity) || identity.Sub != "" {
					t.Fatalf("accepted invalid identity: %+v %v", identity, err)
				}
			} else if err != nil || identity.Sub != "google-sub" || identity.Email != "student@hcmut.edu.vn" {
				t.Fatalf("identity=%+v err=%v", identity, err)
			}
		})
	}
	for _, raw := range []string{"", "bad", "a.b.c"} {
		if _, err := v.Verify(ctx, raw); !errors.Is(err, ErrInvalidIdentity) {
			t.Fatal("accepted malformed token")
		}
	}
	// An algorithm not on the allowlist must be rejected even with a valid signature.
	raw, err := jwt.NewWithClaims(jwt.SigningMethodRS512, valid()).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(ctx, raw); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatal("accepted RS512")
	}
}

func TestMissingClientID(t *testing.T) {
	if _, err := New(context.Background(), " "); err == nil {
		t.Fatal("accepted missing client ID")
	}
}
