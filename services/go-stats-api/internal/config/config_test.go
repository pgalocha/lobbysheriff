package config

import "testing"

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("STATS_DB_NAME", "lobbysheriff_stats")
	t.Setenv("STATS_DB_USER", "stats")
	t.Setenv("STATS_DB_PASSWORD", "secret")
}

func TestLoadDefaults(t *testing.T) {
	setRequired(t)
	for _, k := range []string{"APP_ENV", "GO_STATS_API_PORT", "MYSQL_HOST", "MYSQL_PORT", "REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD"} {
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
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "porta não numérica", env: map[string]string{"GO_STATS_API_PORT": "abc"}},
		{name: "porta fora do intervalo", env: map[string]string{"MYSQL_PORT": "70000"}},
		{name: "password do MySQL em falta", env: map[string]string{"STATS_DB_PASSWORD": ""}},
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
