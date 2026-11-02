package qrtoken

import (
	"strings"
	"testing"
)

func TestTokens(t *testing.T) {
	s, err := New(strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	reg := "11111111-1111-4111-8111-111111111111"
	event := "22222222-2222-4222-8222-222222222222"
	token := s.Sign(reg, event)
	if got, err := s.Verify(token, event); err != nil || got != reg {
		t.Fatalf("%s %v", got, err)
	}
	for _, bad := range []string{"", token + "a", strings.Replace(token, reg, event, 1), strings.Replace(token, "v1.", "v2.", 1)} {
		if _, err := s.Verify(bad, event); err == nil {
			t.Fatal("accepted invalid token")
		}
	}
	if _, err := s.Verify(token, reg); err == nil {
		t.Fatal("accepted wrong event")
	}
	other, _ := New(strings.Repeat("b", 32))
	if _, err := other.Verify(token, event); err == nil {
		t.Fatal("accepted wrong key")
	}
	if _, err := New("short"); err == nil {
		t.Fatal("accepted short key")
	}
}
