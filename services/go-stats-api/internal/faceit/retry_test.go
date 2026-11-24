package faceit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var testRetry = RetryPolicy{MaxRetries: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: 5 * time.Second}

// waitRecorder substitui a espera real: regista as durações e não dorme.
type waitRecorder struct {
	mu     sync.Mutex
	delays []time.Duration
	err    error // devolvido em vez de esperar (simula cancelamento)
}

func (w *waitRecorder) wait(_ context.Context, d time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.delays = append(w.delays, d)
	return w.err
}

func (w *waitRecorder) all() []time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]time.Duration(nil), w.delays...)
}

// retryServer responde com statuses[i] ao pedido i (o último repete-se);
// 200 devolve um jogador. headers aplicam-se às respostas de erro.
func retryServer(t *testing.T, headers map[string]string, statuses ...int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		i := int(calls.Add(1)) - 1
		status := statuses[min(i, len(statuses)-1)]
		if status != http.StatusOK {
			for k, v := range headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write([]byte(`{"player_id":"p1","nickname":"sheriff"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func newRetryClient(t *testing.T, baseURL string, policy RetryPolicy) (*Client, *waitRecorder) {
	t.Helper()
	c, err := New(Options{BaseURL: baseURL, APIKey: testKey, Timeout: time.Second, Retry: policy})
	if err != nil {
		t.Fatal(err)
	}
	rec := &waitRecorder{}
	c.wait = rec.wait
	return c, rec
}

func TestRetrySucceedsAfterTemporaryErrors(t *testing.T) {
	tests := []struct {
		name     string
		statuses []int
		headers  map[string]string
		minWait  time.Duration // espera mínima esperada em cada repetição
	}{
		{name: "429 sem headers espera a janela de 1s", statuses: []int{429, 429, 200}, minWait: time.Second},
		{name: "429 com Retry-After", statuses: []int{429, 200}, headers: map[string]string{"Retry-After": "2"}, minWait: 2 * time.Second},
		{name: "503", statuses: []int{503, 200}},
		{name: "500 e 502", statuses: []int{500, 502, 200}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, calls := retryServer(t, tt.headers, tt.statuses...)
			c, rec := newRetryClient(t, srv.URL, testRetry)

			p, err := c.GetPlayer(t.Context(), "p1")
			if err != nil {
				t.Fatalf("GetPlayer: %v", err)
			}
			if p.Nickname != "sheriff" {
				t.Errorf("player = %+v", p)
			}
			if got, want := int(calls.Load()), len(tt.statuses); got != want {
				t.Errorf("pedidos = %d, esperava %d", got, want)
			}
			delays := rec.all()
			if len(delays) != len(tt.statuses)-1 {
				t.Fatalf("esperas = %v, esperava %d", delays, len(tt.statuses)-1)
			}
			for _, d := range delays {
				if d < tt.minWait {
					t.Errorf("espera %s abaixo do mínimo %s", d, tt.minWait)
				}
			}
		})
	}
}

func TestRetryGivesUp(t *testing.T) {
	srv, calls := retryServer(t, nil, http.StatusServiceUnavailable)
	c, rec := newRetryClient(t, srv.URL, testRetry)

	_, err := c.GetPlayer(t.Context(), "p1")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("erro = %v, esperava ErrUnavailable", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 503 {
		t.Errorf("esperava *APIError 503, obtive %v", err)
	}
	if !strings.Contains(err.Error(), "após 4 tentativas") {
		t.Errorf("erro = %q, esperava indicar as tentativas", err)
	}
	if got := calls.Load(); got != 4 {
		t.Errorf("pedidos = %d, esperava 4 (1 + 3 repetições)", got)
	}

	// Backoff exponencial com jitter: a repetição n espera entre
	// BaseDelay·2ⁿ/2 e BaseDelay·2ⁿ.
	for n, d := range rec.all() {
		full := testRetry.BaseDelay << n
		if d < full/2 || d > full {
			t.Errorf("espera %d = %s, esperava entre %s e %s", n, d, full/2, full)
		}
	}
}

func TestRetryNotForPermanentErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{name: "404", status: http.StatusNotFound, want: ErrNotFound},
		{name: "401", status: http.StatusUnauthorized, want: ErrUnauthorized},
		{name: "400", status: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, calls := retryServer(t, nil, tt.status)
			c, rec := newRetryClient(t, srv.URL, testRetry)

			_, err := c.GetPlayer(t.Context(), "p1")
			if err == nil || (tt.want != nil && !errors.Is(err, tt.want)) {
				t.Fatalf("erro = %v, esperava %v", err, tt.want)
			}
			if calls.Load() != 1 || len(rec.all()) != 0 {
				t.Errorf("pedidos = %d, esperas = %v; não devia repetir", calls.Load(), rec.all())
			}
		})
	}
}

func TestRetryInvalidJSONNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"player_id":`))
	}))
	t.Cleanup(srv.Close)
	c, _ := newRetryClient(t, srv.URL, testRetry)

	if _, err := c.GetPlayer(t.Context(), "p1"); err == nil {
		t.Fatal("esperava erro")
	}
	if calls.Load() != 1 {
		t.Errorf("pedidos = %d; resposta inválida não se repete", calls.Load())
	}
}

func TestRetryNetworkErrors(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // ligação recusada em todos os pedidos

	c, rec := newRetryClient(t, url, testRetry)
	_, err := c.GetPlayer(t.Context(), "p1")
	if err == nil || !strings.Contains(err.Error(), "após 4 tentativas") {
		t.Fatalf("erro = %v, esperava falha de rede após 4 tentativas", err)
	}
	if len(rec.all()) != 3 {
		t.Errorf("esperas = %v, esperava 3", rec.all())
	}
}

func TestRetryAfterAboveMaxDelayGivesUp(t *testing.T) {
	srv, calls := retryServer(t, map[string]string{"Retry-After": "60"}, http.StatusTooManyRequests)
	c, rec := newRetryClient(t, srv.URL, testRetry) // MaxDelay 5s

	_, err := c.GetPlayer(t.Context(), "p1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RetryAfter != time.Minute {
		t.Fatalf("erro = %v, esperava APIError com RetryAfter 1m", err)
	}
	if calls.Load() != 1 || len(rec.all()) != 0 {
		t.Errorf("pedidos = %d, esperas = %v; devia desistir logo", calls.Load(), rec.all())
	}
}

func TestRetryCanceledWhileWaiting(t *testing.T) {
	srv, calls := retryServer(t, nil, http.StatusTooManyRequests)
	c, rec := newRetryClient(t, srv.URL, testRetry)
	rec.err = context.Canceled

	_, err := c.GetPlayer(t.Context(), "p1")
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrRateLimited) {
		t.Fatalf("erro = %v, esperava Canceled e ErrRateLimited", err)
	}
	if calls.Load() != 1 {
		t.Errorf("pedidos = %d, esperava 1", calls.Load())
	}
}

func TestRetryPolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		policy  RetryPolicy
		wantErr bool
	}{
		{name: "sem retry", policy: RetryPolicy{}},
		{name: "por omissão", policy: DefaultRetryPolicy},
		{name: "negativo", policy: RetryPolicy{MaxRetries: -1}, wantErr: true},
		{name: "sem BaseDelay", policy: RetryPolicy{MaxRetries: 1, MaxDelay: time.Second}, wantErr: true},
		{name: "MaxDelay < BaseDelay", policy: RetryPolicy{MaxRetries: 1, BaseDelay: time.Second, MaxDelay: time.Millisecond}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(Options{BaseURL: "https://x.test", APIKey: "k", Timeout: time.Second, Retry: tt.policy})
			if (err != nil) != tt.wantErr {
				t.Errorf("New() erro = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBackoffCappedAtMaxDelay(t *testing.T) {
	p := RetryPolicy{MaxRetries: 50, BaseDelay: time.Second, MaxDelay: 4 * time.Second}
	for _, n := range []int{3, 10, 40} {
		if d := p.backoff(n); d < 2*time.Second || d > 4*time.Second {
			t.Errorf("backoff(%d) = %s, esperava entre 2s e 4s", n, d)
		}
	}
}

func TestSleepRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start := time.Now()
	if err := sleep(ctx, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v", err)
	}
	if time.Since(start) > time.Second {
		t.Error("sleep não respeitou o cancelamento")
	}
}
