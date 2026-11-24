// Package server monta a aplicação HTTP (Fiber) do go-stats-api.
package server

import (
	"net/http"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/healthcheck"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/health"
)

// New cria a aplicação Fiber com as rotas do serviço.
func New(checker *health.Checker) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "go-stats-api",
	})

	app.Use(recover.New())

	// Liveness: só o processo. Não depende do MySQL/Redis, senão o Kubernetes
	// reiniciava a app sempre que uma dependência falhasse.
	app.Get(healthcheck.LivenessEndpoint, healthcheck.New())

	// Readiness: pronto para receber tráfego (todas as dependências disponíveis).
	app.Get(healthcheck.ReadinessEndpoint, healthcheck.New(healthcheck.Config{
		Probe: func(c fiber.Ctx) bool {
			return checker.Run(c.Context()).Healthy()
		},
	}))

	// Estado detalhado de cada dependência, em JSON.
	app.Get("/health", func(c fiber.Ctx) error {
		report := checker.Run(c.Context())
		status := http.StatusOK
		if !report.Healthy() {
			status = http.StatusServiceUnavailable
		}
		return c.Status(status).JSON(report)
	})

	return app
}
