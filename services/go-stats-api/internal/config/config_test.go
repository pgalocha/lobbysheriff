package config

import (
	"testing"
	"time"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("STATS_DB_NAME", "lobbysheriff_stats")
	t.Setenv("STATS_DB_USER", "stats")
	t.Setenv("STATS_DB_PASSWORD", "secret")
}

func TestLoadDefaults(t *testing.T) {
	setRequired(t)
	for _, k := range []string{"APP_ENV", "GO_STATS_API_PORT", "MYSQL_HOST", "MYSQL_PORT", "REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD", "CORS_ALLOWED_ORIGINS", "FACEIT_API_KEY", "FACEIT_API_BASE_URL", "FACEIT_HTTP_TIMEOUT"} {
		t.Setenv(k, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if cfg.Env != "local" || cfg.Port != 8080 {
		t.Errorf("app = %q:%d, esperava local:8080", cfg.Env, cfg.Port)
	}
	if got := cfg.MySQL.Addr(); got != "mysql:3306" {
		t.Errorf("MySQL.Addr() = %q, esperava mysql:3306", got)
	}
	if got := cfg.Redis.Addr(); got != "redis:6379" {
		t.Errorf("Redis.Addr() = %q, esperava redis:6379", got)
	}
	if cfg.CORSAllowedOrigins != nil {
		t.Errorf("CORSAllowedOrigins = %v, esperava vazio", cfg.CORSAllowedOrigins)
	}
	if cfg.FACEIT.BaseURL != "https://open.faceit.com/data/v4" || cfg.FACEIT.Timeout != 10*time.Second {
		t.Errorf("FACEIT = %q (timeout %s), esperava a URL pública e 10s", cfg.FACEIT.BaseURL, cfg.FACEIT.Timeout)
	}
}

func TestFACEITValidate(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "com key", key: "test-key", wantErr: false},
		{name: "sem key", key: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv("FACEIT_API_KEY", tt.key)

			// O Load não exige a key (migrate/healthcheck não a usam).
			cfg, err := Load()
			if err != nil {
				t.Fatalf("erro inesperado no Load: %v", err)
			}
			if err := cfg.FACEIT.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() erro = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadFACEITTimeout(t *testing.T) {
	setRequired(t)
	t.Setenv("FACEIT_HTTP_TIMEOUT", "1m30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if cfg.FACEIT.Timeout != 90*time.Second {
		t.Errorf("Timeout = %s, esperava 1m30s", cfg.FACEIT.Timeout)
	}
}

func TestLoadCORSOrigins(t *testing.T) {
	setRequired(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", " http://localhost:8090 ,, http://localhost:5173")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	want := []string{"http://localhost:8090", "http://localhost:5173"}
	if len(cfg.CORSAllowedOrigins) != 2 || cfg.CORSAllowedOrigins[0] != want[0] || cfg.CORSAllowedOrigins[1] != want[1] {
		t.Errorf("CORSAllowedOrigins = %q, esperava %q", cfg.CORSAllowedOrigins, want)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "porta não numérica", env: map[string]string{"GO_STATS_API_PORT": "abc"}},
		{name: "porta fora do intervalo", env: map[string]string{"MYSQL_PORT": "70000"}},
		{name: "password do MySQL em falta", env: map[string]string{"STATS_DB_PASSWORD": ""}},
		{name: "timeout da FACEIT inválido", env: map[string]string{"FACEIT_HTTP_TIMEOUT": "dez"}},
		{name: "timeout da FACEIT não positivo", env: map[string]string{"FACEIT_HTTP_TIMEOUT": "0s"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequired(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if cfg, err := Load(); err == nil {
				t.Fatalf("esperava erro, obtive %+v", cfg)
			}
		})
	}
}
