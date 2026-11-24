package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"

	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/api"
)

// contractRouter cria o router do kin-openapi para o contrato, ignorando os
// `servers` (o mesmo contrato serve para localhost, staging e produção).
func contractRouter(doc *openapi3.T) (routers.Router, error) {
	doc.Servers = nil
	if err := doc.Validate(openapi3.NewLoader().Context); err != nil {
		return nil, fmt.Errorf("contrato OpenAPI inválido: %w", err)
	}
	return gorillamux.NewRouter(doc)
}

// validateRequests rejeita com 400 os pedidos que não cumprem o contrato
// (parâmetros, query string, headers e corpo). Rotas fora do contrato seguem
// para o Fiber, que responde 404/405.
func validateRequests(router routers.Router) fiber.Handler {
	options := &openapi3filter.Options{
		MultiError: true,
		// A autenticação ainda não está definida no contrato; quando estiver,
		// troca-se por um AuthenticationFunc real.
		AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
	}

	return func(c fiber.Ctx) error {
		req, err := adaptor.ConvertRequest(c, false)
		if err != nil {
			return fmt.Errorf("converter pedido: %w", err)
		}

		route, pathParams, err := router.FindRoute(req)
		if err != nil {
			return c.Next()
		}

		input := &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
			Options:    options,
		}
		if err := openapi3filter.ValidateRequest(c.Context(), input); err != nil {
			details := validationDetails(err)
			return c.Status(http.StatusBadRequest).JSON(api.Error{
				Error:   "o pedido não cumpre o contrato da API",
				Details: &details,
			})
		}
		return c.Next()
	}
}

func validationDetails(err error) []string {
	var multi openapi3.MultiError
	if errors.As(err, &multi) {
		details := make([]string, 0, len(multi))
		for _, e := range multi {
			details = append(details, e.Error())
		}
		return details
	}
	return []string{err.Error()}
}

// errorHandler devolve os erros no formato Error do contrato.
func errorHandler(c fiber.Ctx, err error) error {
	status := http.StatusInternalServerError
	msg := http.StatusText(status)

	var fe *fiber.Error
	if errors.As(err, &fe) {
		status = fe.Code
		msg = fe.Message
	}
	return c.Status(status).JSON(api.Error{Error: msg})
}
