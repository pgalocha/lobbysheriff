package faceit

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"time"
)

// RetryPolicy define as novas tentativas com backoff exponencial.
//
// Só se repetem erros temporários: 429 (rate limit), 5xx e erros de rede.
// A espera da tentativa n (0, 1, 2, ...) é BaseDelay·2ⁿ, limitada a MaxDelay,
// com jitter (entre metade e o valor total), para vários clientes não
// repetirem em simultâneo. Se a FACEIT indicar quanto esperar (Retry-After /
// RateLimit-Reset), espera-se pelo menos isso; se for mais do que MaxDelay,
// desiste-se logo e quem chamou decide (ex.: o sync tenta no ciclo seguinte).
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultRetryPolicy é a política de produção: até 3 novas tentativas, com
// esperas de ~0,5s, 1s e 2s (mais, se a FACEIT o pedir).
var DefaultRetryPolicy = RetryPolicy{MaxRetries: 3, BaseDelay: 500 * time.Millisecond, MaxDelay: 30 * time.Second}

func (p RetryPolicy) validate() error {
	switch {
	case p.MaxRetries < 0:
		return errors.New("faceit: MaxRetries não pode ser negativo")
	case p.MaxRetries == 0:
		return nil
	case p.BaseDelay <= 0 || p.MaxDelay < p.BaseDelay:
		return errors.New("faceit: retry inválido (BaseDelay > 0 e MaxDelay >= BaseDelay)")
	}
	return nil
}

// backoff devolve a espera antes da nova tentativa n (0 = primeira repetição).
func (p RetryPolicy) backoff(n int) time.Duration {
	d := p.MaxDelay
	if n < 30 { // evita overflow no deslocamento
		if b := p.BaseDelay << n; b > 0 && b < p.MaxDelay {
			d = b
		}
	}
	// Jitter em [d/2, d]. Não é uso criptográfico.
	half := d / 2
	return half + rand.N(d-half+1) //nolint:gosec // jitter de backoff, não precisa de aleatoriedade segura
}

// get faz o pedido com novas tentativas segundo a RetryPolicy do Client.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	for attempt := 0; ; attempt++ {
		err := c.do(ctx, path, query, out)
		if err == nil {
			return nil
		}
		if attempt >= c.retry.MaxRetries || !retryable(err) || ctx.Err() != nil {
			return withAttempts(err, attempt+1)
		}

		delay := c.retry.backoff(attempt)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.RetryAfter > delay {
			if apiErr.RetryAfter > c.retry.MaxDelay {
				return withAttempts(err, attempt+1)
			}
			delay = apiErr.RetryAfter
		}
		if werr := c.wait(ctx, delay); werr != nil {
			return fmt.Errorf("%w (cancelado antes da tentativa %d: %w)", err, attempt+2, werr)
		}
	}
}

// retryable: erros temporários, em que repetir pode resultar.
func retryable(err error) bool {
	var te *transportError
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrUnavailable) || errors.As(err, &te)
}

func withAttempts(err error, n int) error {
	if n <= 1 {
		return err
	}
	return fmt.Errorf("%w (após %d tentativas)", err, n)
}

// transportError marca falhas de rede (ligação recusada, timeout, ...), para
// as distinguir de respostas inválidas, que não vale a pena repetir.
type transportError struct{ err error }

func (e *transportError) Error() string { return e.err.Error() }
func (e *transportError) Unwrap() error { return e.err }

// sleep espera d ou até o contexto terminar.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
