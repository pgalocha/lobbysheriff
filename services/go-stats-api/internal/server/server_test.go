package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/health"
)

func newChecker(redisErr error) *health.Checker {
	c := health.New(time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.Add("mysql", func(context.Context) error { return nil })
	c.Add("redis", func(context.Context) error { return redisErr })
	return c
}

func get(t *testing.T, checker *health.Checker, path string) (int, []byte) {
	t.Helper()
	resp, err := New(checker).Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody))
	if err != nil {
		t.Fatalf("pedido falhou: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("leitura do corpo falhou: %v", err)
	}
	return resp.StatusCode, body
}

func TestRoutes(t *testing.T) {
	down := errors.New("connection refused")

	tests := []struct {
		name     string
		path     string
		redisErr error
		want     int
	}{
		{name: "livez com dependências ok", path: "/livez", want: http.StatusOK},
		{name: "livez ignora dependências", path: "/livez", redisErr: down, want: http.StatusOK},
		{name: "readyz com dependências ok", path: "/readyz", want: http.StatusOK},
		{name: "readyz com redis em baixo", path: "/readyz", redisErr: down, want: http.StatusServiceUnavailable},
		{name: "health com dependências ok", path: "/health", want: http.StatusOK},
		{name: "health com redis em baixo", path: "/health", redisErr: down, want: http.StatusServiceUnavailable},
		{name: "rota inexistente", path: "/nao-existe", want: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := get(t, newChecker(tt.redisErr), tt.path)
			if got != tt.want {
				t.Errorf("GET %s = %d, esperava %d", tt.path, got, tt.want)
			}
		})
	}
}

func TestHealthBody(t *testing.T) {
	_, body := get(t, newChecker(errors.New("connection refused")), "/health")

	var report health.Report
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("JSON inválido %q: %v", body, err)
	}
	if report.Status != health.StatusDown {
		t.Errorf("status = %q, esperava %q", report.Status, health.StatusDown)
	}
	if report.Checks["mysql"].Status != health.StatusUp || report.Checks["redis"].Status != health.StatusDown {
		t.Errorf("checks inesperados: %+v", report.Checks)
	}
}
