// Package server monta a aplicação HTTP (Fiber) do go-stats-api.
//
// As rotas vêm do contrato OpenAPI (contracts/openapi/go-stats-api.yaml),
// através do código gerado em internal/api; ver handlers.go.
package server

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/api"
	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/health"
)

// Options configura a aplicação HTTP.
type Options struct {
	// CORSAllowedOrigins: origens autorizadas a chamar a API a partir do
	// browser (ex.: Swagger UI local). Vazio = CORS desativado.
	CORSAllowedOrigins []string
}

// New cria a aplicação Fiber com as rotas do contrato e a validação de pedidos.
func New(checker *health.Checker, opts Options) (*fiber.App, error) {
	doc, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("carregar contrato OpenAPI: %w", err)
	}
	router, err := contractRouter(doc)
	if err != nil {
		return nil, err
	}

	app := fiber.New(fiber.Config{
		AppName:      "go-stats-api",
		ErrorHandler: errorHandler,
	})

	app.Use(recover.New())
	if len(opts.CORSAllowedOrigins) > 0 {
		app.Use(cors.New(cors.Config{AllowOrigins: opts.CORSAllowedOrigins}))
	}
	app.Use(validateRequests(router))

	api.RegisterHandlers(app, &handlers{checker: checker})

	return app, nil
}
