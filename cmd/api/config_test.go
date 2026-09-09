package main

import (
	"strings"
	"testing"
	"time"
)

var configEnvironment = []string{
	"DATABASE_URL", "PORT", "CORS_ORIGINS", "ENRICHMENT_ENABLED", "HANDY_API_KEY",
	"HANDY_API_MONTHLY_LIMIT", "RATE_LIMIT_RPS", "RATE_LIMIT_BURST",
	"GLOBAL_RATE_LIMIT_RPS", "GLOBAL_RATE_LIMIT_BURST", "RATE_LIMIT_MAX_CLIENTS",
	"DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME", "DB_CONN_MAX_IDLE_TIME",
	"HTTP_READ_HEADER_TIMEOUT", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT",
	"HTTP_SHUTDOWN_TIMEOUT", "HTTP_MAX_HEADER_BYTES",
}

func TestLoadConfigDefaultsAndOverrides(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://test")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	if cfg.port != 8080 || cfg.dbMaxOpenConns != 20 || cfg.writeTimeout != 30*time.Second || cfg.maxHeaderBytes != 1<<20 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}

	t.Setenv("PORT", "9000")
	t.Setenv("ENRICHMENT_ENABLED", "true")
	t.Setenv("DB_MAX_OPEN_CONNS", "12")
	t.Setenv("DB_MAX_IDLE_CONNS", "4")
	t.Setenv("HTTP_WRITE_TIMEOUT", "45s")
	cfg, err = loadConfig()
	if err != nil {
		t.Fatalf("load overridden config: %v", err)
	}
	if cfg.port != 9000 || !cfg.enrichmentEnabled || cfg.dbMaxOpenConns != 12 || cfg.dbMaxIdleConns != 4 || cfg.writeTimeout != 45*time.Second {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
}

func TestLoadConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr string
	}{
		{name: "missing database", wantErr: "DATABASE_URL"},
		{name: "port range", key: "PORT", value: "70000", wantErr: "PORT"},
		{name: "boolean", key: "ENRICHMENT_ENABLED", value: "sometimes", wantErr: "boolean"},
		{name: "integer", key: "DB_MAX_OPEN_CONNS", value: "0", wantErr: "positive integer"},
		{name: "idle exceeds open", key: "DB_MAX_IDLE_CONNS", value: "21", wantErr: "must not exceed"},
		{name: "duration", key: "HTTP_IDLE_TIMEOUT", value: "later", wantErr: "Go duration"},
		{name: "float", key: "RATE_LIMIT_RPS", value: "NaN", wantErr: "positive number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnvironment(t)
			if tt.name != "missing database" {
				t.Setenv("DATABASE_URL", "postgres://test")
			}
			if tt.key != "" {
				t.Setenv(tt.key, tt.value)
			}
			_, err := loadConfig()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range configEnvironment {
		t.Setenv(name, "")
	}
}
