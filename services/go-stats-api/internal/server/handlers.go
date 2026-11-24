package server

import (
	"net/http"

	"github.com/gofiber/fiber/v3"

	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/api"
	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/health"
)

// handlers implementa as operações do contrato OpenAPI.
type handlers struct {
	checker *health.Checker
}

// Falha a compilação se faltar alguma operação do contrato.
var _ api.ServerInterface = (*handlers)(nil)

func (h *handlers) GetLiveness(c fiber.Ctx) error {
	return c.SendString("OK")
}

func (h *handlers) GetReadiness(c fiber.Ctx) error {
	if !h.checker.Run(c.Context()).Healthy() {
		return c.Status(http.StatusServiceUnavailable).SendString("Service Unavailable")
	}
	return c.SendString("OK")
}

func (h *handlers) GetHealth(c fiber.Ctx) error {
	report := h.checker.Run(c.Context())

	body := api.HealthReport{
		Status: toAPIStatus(report.Status),
		Checks: make(map[string]api.HealthCheck, len(report.Checks)),
	}
	for name, res := range report.Checks {
		body.Checks[name] = api.HealthCheck{Status: toAPIStatus(res.Status), LatencyMs: res.LatencyMS}
	}

	status := http.StatusOK
	if !report.Healthy() {
		status = http.StatusServiceUnavailable
	}
	return c.Status(status).JSON(body)
}

func toAPIStatus(s string) api.HealthStatus {
	if s == health.StatusUp {
		return api.Up
	}
	return api.Down
}
