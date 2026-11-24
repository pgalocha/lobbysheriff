package health

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func newTestChecker(timeout time.Duration) *Checker {
	return New(timeout, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func ok(context.Context) error { return nil }

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		checks     map[string]CheckFunc
		wantStatus string
		wantChecks map[string]string
	}{
		{
			name:       "todas as dependências disponíveis",
			checks:     map[string]CheckFunc{"mysql": ok, "redis": ok},
			wantStatus: StatusUp,
			wantChecks: map[string]string{"mysql": StatusUp, "redis": StatusUp},
		},
		{
			name: "uma dependência em baixo",
			checks: map[string]CheckFunc{
				"mysql": ok,
				"redis": func(context.Context) error { return errors.New("connection refused") },
			},
			wantStatus: StatusDown,
			wantChecks: map[string]string{"mysql": StatusUp, "redis": StatusDown},
		},
		{
			name:       "sem verificações",
			checks:     map[string]CheckFunc{},
			wantStatus: StatusUp,
			wantChecks: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestChecker(time.Second)
			for name, fn := range tt.checks {
				c.Add(name, fn)
			}

			report := c.Run(t.Context())

			if report.Status != tt.wantStatus {
				t.Errorf("status = %q, esperava %q", report.Status, tt.wantStatus)
			}
			if len(report.Checks) != len(tt.wantChecks) {
				t.Fatalf("checks = %v, esperava %v", report.Checks, tt.wantChecks)
			}
			for name, want := range tt.wantChecks {
				if got := report.Checks[name].Status; got != want {
					t.Errorf("check %q = %q, esperava %q", name, got, want)
				}
			}
		})
	}
}

func TestRunTimeout(t *testing.T) {
	c := newTestChecker(20 * time.Millisecond)
	c.Add("lento", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	start := time.Now()
	report := c.Run(t.Context())

	if report.Healthy() {
		t.Error("esperava relatório em baixo após timeout")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Run demorou %v; o timeout não foi respeitado", elapsed)
	}
}
