// Package google verifies school Google ID tokens; it never stores Google tokens.
package google

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
)

const schoolDomain = "hcmut.edu.vn"

var ErrInvalidIdentity = errors.New("invalid Google identity")

type Verifier struct{ verifier *oidc.IDTokenVerifier }

var _ application.IdentityVerifier = (*Verifier)(nil)

// New performs discovery once during startup. ctx must outlive requests: the
// provider's remote key set retains it for subsequent JWKS refreshes.
func New(ctx context.Context, clientID string) (*Verifier, error) {
	if strings.TrimSpace(clientID) == "" {
		return nil, errors.New("GOOGLE_CLIENT_ID is required")
	}
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, fmt.Errorf("discover Google OIDC provider: %w", err)
	}
	return &Verifier{verifier: provider.Verifier(&oidc.Config{ClientID: clientID, SupportedSigningAlgs: []string{oidc.RS256}})}, nil
}

func (v *Verifier) Verify(ctx context.Context, raw string) (application.Identity, error) {
	if strings.TrimSpace(raw) == "" {
		return application.Identity{}, ErrInvalidIdentity
	}
	token, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		if ctx.Err() != nil {
			return application.Identity{}, ctx.Err()
		}
		return application.Identity{}, ErrInvalidIdentity
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		HostedDomain  string `json:"hd"`
		Name          string `json:"name"`
	}
	if err := token.Claims(&claims); err != nil {
		return application.Identity{}, ErrInvalidIdentity
	}
	address, err := mail.ParseAddress(claims.Email)
	local, domain, ok := strings.Cut(claims.Email, "@")
	if err != nil || address.Address != claims.Email || !ok || local == "" || domain != schoolDomain || !claims.EmailVerified || claims.HostedDomain != schoolDomain || strings.TrimSpace(token.Subject) == "" {
		return application.Identity{}, ErrInvalidIdentity
	}
	return application.Identity{Sub: token.Subject, Email: claims.Email, FullName: claims.Name}, nil
}
