package qrtoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	version = "v1"
	// macContext gives the MAC domain separation so the key cannot be confused
	// with other HMAC uses even if it is ever shared.
	macContext = "registration-qr.v1"
	uuidLength = 36
)

var errInvalid = errors.New("invalid QR")

// Service signs opaque registration/event identifiers, never personal data.
type Service struct{ key []byte }

func New(secret string) (*Service, error) {
	if len(strings.TrimSpace(secret)) < 32 {
		return nil, errors.New("QR_HMAC_SECRET must contain at least 32 nonblank bytes")
	}
	return &Service{key: []byte(secret)}, nil
}

func (s *Service) mac(registration, event string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(macContext + "|" + registration + "|" + event))
	return m.Sum(nil)
}

func (s *Service) Sign(registration, event string) string {
	return version + "." + registration + "." + event + "." + hex.EncodeToString(s.mac(registration, event))
}

func (s *Service) Verify(token, event string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 4 || parts[0] != version || parts[2] != event || len(parts[1]) != uuidLength {
		return "", errInvalid
	}
	signature, err := hex.DecodeString(parts[3])
	if err != nil || !hmac.Equal(signature, s.mac(parts[1], parts[2])) {
		return "", errInvalid
	}
	return parts[1], nil
}
