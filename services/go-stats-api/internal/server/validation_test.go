package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gofiber/fiber/v3"

	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/api"
)

// Contrato de exemplo com parâmetros, para testar o middleware de validação
// independentemente das rotas reais.
const exampleContract = `
openapi: 3.0.3
info: {title: exemplo, version: "1"}
paths:
  /players/{id}:
    get:
      operationId: getPlayer
      parameters:
        - {name: id, in: path, required: true, schema: {type: integer, minimum: 1}}
        - {name: limit, in: query, schema: {type: integer, minimum: 1, maximum: 10}}
      responses:
        "200": {description: ok}
`

func newValidatedApp(t *testing.T) *fiber.App {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData([]byte(exampleContract))
	if err != nil {
		t.Fatal(err)
	}
	router, err := contractRouter(doc)
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New(fiber.Config{ErrorHandler: errorHandler})
	app.Use(validateRequests(router))
	app.Get("/players/:id", func(c fiber.Ctx) error { return c.SendString("ok") })
	return app
}

func TestValidateRequests(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		want        int
		wantDetails bool
	}{
		{name: "pedido válido", path: "/players/42?limit=5", want: http.StatusOK},
		{name: "path param não numérico", path: "/players/abc", want: http.StatusBadRequest, wantDetails: true},
		{name: "path param abaixo do mínimo", path: "/players/0", want: http.StatusBadRequest, wantDetails: true},
		{name: "query acima do máximo", path: "/players/42?limit=50", want: http.StatusBadRequest, wantDetails: true},
		{name: "rota fora do contrato segue para o Fiber", path: "/outra", want: http.StatusNotFound},
	}

	app := newValidatedApp(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, http.NoBody)
			res := do(t, app, req)

			if res.status != tt.want {
				t.Fatalf("GET %s = %d, esperava %d (corpo: %s)", tt.path, res.status, tt.want, res.body)
			}
			if !tt.wantDetails {
				return
			}
			var body api.Error
			if err := json.Unmarshal(res.body, &body); err != nil {
				t.Fatalf("corpo não é um Error: %s", res.body)
			}
			if body.Details == nil || len(*body.Details) == 0 {
				t.Errorf("esperava detalhes da validação, obtive %s", res.body)
			}
		})
	}
}
