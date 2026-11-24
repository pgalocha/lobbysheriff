// Package health verifica o estado das dependências do serviço (MySQL, Redis, …).
package health

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Estados possíveis de uma verificação e do relatório global.
const (
	StatusUp   = "up"
	StatusDown = "down"
)

// CheckFunc verifica uma dependência; devolve nil se estiver saudável.
type CheckFunc func(ctx context.Context) error

// Result é o resultado de uma verificação individual.
type Result struct {
	Status    string
	LatencyMS int64
}

// Report agrega os resultados de todas as verificações.
type Report struct {
	Status string
	Checks map[string]Result
}

// Healthy indica se todas as dependências estão disponíveis.
func (r Report) Healthy() bool {
	return r.Status == StatusUp
}

type check struct {
	name string
	fn   CheckFunc
}

// Checker corre as verificações registadas, em paralelo e com timeout.
type Checker struct {
	timeout time.Duration
	logger  *slog.Logger
	checks  []check
}

// New cria um Checker em que cada verificação tem no máximo `timeout` para responder.
func New(timeout time.Duration, logger *slog.Logger) *Checker {
	return &Checker{timeout: timeout, logger: logger}
}

// Add regista uma verificação com o nome indicado.
func (c *Checker) Add(name string, fn CheckFunc) {
	c.checks = append(c.checks, check{name: name, fn: fn})
}

// Run executa todas as verificações e devolve o relatório.
// Os detalhes dos erros vão para o log e não para a resposta, para não expor
// informação interna (hosts, utilizadores) a quem chama o endpoint.
func (c *Checker) Run(ctx context.Context) Report {
	report := Report{Status: StatusUp, Checks: make(map[string]Result, len(c.checks))}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for _, ch := range c.checks {
		wg.Go(func() {
			checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
			defer cancel()

			start := time.Now()
			err := ch.fn(checkCtx)
			res := Result{Status: StatusUp, LatencyMS: time.Since(start).Milliseconds()}
			if err != nil {
				res.Status = StatusDown
				c.logger.Warn("health check falhou", "check", ch.name, "error", err)
			}

			mu.Lock()
			defer mu.Unlock()
			report.Checks[ch.name] = res
			if err != nil {
				report.Status = StatusDown
			}
		})
	}
	wg.Wait()

	return report
}
