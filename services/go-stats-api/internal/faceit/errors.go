package faceit

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Erros sentinela, para usar com errors.Is.
var (
	// ErrNotFound: o recurso não existe na FACEIT (404).
	ErrNotFound = errors.New("faceit: não encontrado")
	// ErrUnauthorized: API key inválida ou sem permissão (401/403).
	ErrUnauthorized = errors.New("faceit: não autorizado")
	// ErrRateLimited: limite de pedidos excedido (429). Ver APIError.RetryAfter.
	ErrRateLimited = errors.New("faceit: limite de pedidos excedido")
	// ErrUnavailable: erro do lado da FACEIT (5xx); pode tentar-se mais tarde.
	ErrUnavailable = errors.New("faceit: serviço indisponível")
)

// APIError é uma resposta da FACEIT com status diferente de 200.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	// RetryAfter vem do header Retry-After (429/503); zero se ausente.
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("faceit: %s %s: status %d", e.Method, e.Path, e.StatusCode)
}

// Unwrap devolve o erro sentinela correspondente ao status, para errors.Is.
func (e *APIError) Unwrap() error {
	switch {
	case e.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case e.StatusCode == http.StatusTooManyRequests:
		return ErrRateLimited
	case e.StatusCode >= 500:
		return ErrUnavailable
	default:
		return nil
	}
}

// parseRetryAfter aceita segundos ("30") ou uma data HTTP; devolve zero se
// o valor estiver ausente, for inválido ou já tiver passado.
func parseRetryAfter(v string, now time.Time) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil {
		if s < 0 {
			return 0
		}
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}
