package config

import "testing"

func TestLoad(t *testing.T) {
	for _, tt := range []struct {
		name, addr, database string
		wantErr              bool
	}{
		{"defaults", "", "postgres://localhost/ticket", false},
		{"explicit address", "127.0.0.1:4000", "postgres://localhost/ticket", false},
		{"missing database", "", "", true},
		{"invalid address", "invalid", "postgres://localhost/ticket", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HTTP_ADDR", tt.addr)
			t.Setenv("DATABASE_URL", tt.database)
			cfg, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.addr == "" && cfg.HTTPAddr != ":3000" {
				t.Fatalf("unexpected default: %s", cfg.HTTPAddr)
			}
		})
	}
}
