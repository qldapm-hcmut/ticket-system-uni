package config

import "testing"

func TestLoadGoogleClientID(t *testing.T) {
	for _, id := range []string{"", " ", " padded "} {
		t.Setenv("GOOGLE_CLIENT_ID", id)
		if _, err := LoadGoogleClientID(); err == nil {
			t.Fatal("accepted invalid client ID")
		}
	}
	t.Setenv("GOOGLE_CLIENT_ID", "test.apps.googleusercontent.com")
	if id, err := LoadGoogleClientID(); err != nil || id != "test.apps.googleusercontent.com" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}
