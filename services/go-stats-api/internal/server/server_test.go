package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/gofiber/fiber/v3"

	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/api"
	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/health"
)

func newChecker(redisErr error) *health.Checker {
	c := health.New(time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.Add("mysql", func(context.Context) error { return nil })
	c.Add("redis", func(context.Context) error { return redisErr })
	return c
}

func newApp(t *testing.T, redisErr error, opts Options) *fiber.App {
	t.Helper()
	app, err := New(newChecker(redisErr), opts)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return app
}

func newContractRouter(t *testing.T) routers.Router {
	t.Helper()
	doc, err := api.GetSpec()
	if err != nil {
		t.Fatal(err)
	}
	router, err := contractRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func do(t *testing.T, app *fiber.App, req *http.Request) response {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("pedido falhou: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("leitura do corpo falhou: %v", err)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: body}
}

// assertMatchesContract valida a resposta (status, content-type e corpo)
// contra o contrato OpenAPI. Status não documentados também falham.
// Devolve a chave "<operationId> <status>" da resposta validada.
func assertMatchesContract(t *testing.T, router routers.Router, req *http.Request, res response) string {
	t.Helper()
	route, params, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("%s %s não está no contrato: %v", req.Method, req.URL.Path, err)
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route},
		Status:                 res.status,
		Header:                 res.header,
		Body:                   io.NopCloser(bytes.NewReader(res.body)),
		Options:                &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
	}
	if err := openapi3filter.ValidateResponse(t.Context(), input); err != nil {
		t.Errorf("resposta de %s %s (%d) não cumpre o contrato:\n%v\ncorpo: %s",
			req.Method, req.URL.Path, res.status, err, res.body)
	}
	return fmt.Sprintf("%s %d", route.Operation.OperationID, res.status)
}

// assertAllResponsesCovered falha se alguma resposta documentada no contrato
// (operação + status) não tiver um caso em TestResponsesMatchContract.
// Status em intervalo (ex.: "4XX") ficam cobertos por qualquer status desse
// intervalo; "default" não é exigido.
func assertAllResponsesCovered(t *testing.T, covered map[string]bool) {
	t.Helper()
	doc, err := api.GetSpec()
	if err != nil {
		t.Fatal(err)
	}

	coveredRange := map[string]bool{}
	for key := range covered {
		op, status, _ := strings.Cut(key, " ")
		coveredRange[op+" "+status[:1]+"XX"] = true
	}

	var missing []string
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			for status := range op.Responses.Map() {
				key := op.OperationID + " " + status
				switch {
				case status == "default":
				case strings.HasSuffix(strings.ToUpper(status), "XX"):
					if !coveredRange[op.OperationID+" "+strings.ToUpper(status)] {
						missing = append(missing, fmt.Sprintf("%s (%s %s → %s)", key, method, path, status))
					}
				case !covered[key]:
					missing = append(missing, fmt.Sprintf("%s (%s %s → %s)", key, method, path, status))
				}
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("respostas do contrato sem caso em TestResponsesMatchContract:\n  %s\n"+
			"acrescenta um caso por cada uma (ver contracts/README.md)", strings.Join(missing, "\n  "))
	}
}

// Cada resposta documentada no contrato (operação + status, incluindo os de
// erro) tem de ter aqui pelo menos um caso; o teste falha se faltar algum.
func TestResponsesMatchContract(t *testing.T) {
	down := errors.New("connection refused")
	router := newContractRouter(t)
	covered := map[string]bool{}

	tests := []struct {
		name     string
		method   string // por omissão GET
		path     string
		body     string // JSON; vazio = sem corpo
		redisErr error
		want     int
	}{
		{name: "livez", path: "/livez", want: http.StatusOK},
		{name: "livez ignora dependências", path: "/livez", redisErr: down, want: http.StatusOK},
		{name: "readyz ok", path: "/readyz", want: http.StatusOK},
		{name: "readyz com redis em baixo", path: "/readyz", redisErr: down, want: http.StatusServiceUnavailable},
		{name: "health ok", path: "/health", want: http.StatusOK},
		{name: "health com redis em baixo", path: "/health", redisErr: down, want: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			newReq := func() *http.Request {
				var body io.Reader = http.NoBody
				if tt.body != "" {
					body = strings.NewReader(tt.body)
				}
				req := httptest.NewRequestWithContext(t.Context(), method, tt.path, body)
				if tt.body != "" {
					req.Header.Set("Content-Type", "application/json")
				}
				return req
			}

			res := do(t, newApp(t, tt.redisErr, Options{}), newReq())
			if res.status != tt.want {
				t.Fatalf("%s %s = %d, esperava %d (corpo: %s)", method, tt.path, res.status, tt.want, res.body)
			}
			// Pedido novo: o corpo do primeiro já foi consumido pela app.
			covered[assertMatchesContract(t, router, newReq(), res)] = true
		})
	}

	assertAllResponsesCovered(t, covered)
}

var pathParam = regexp.MustCompile(`\{([^}]+)\}`)

// Todas as operações do contrato estão implementadas e não há rotas fora dele.
func TestRoutesMatchContract(t *testing.T) {
	doc, err := api.GetSpec()
	if err != nil {
		t.Fatal(err)
	}

	var inContract []string
	for path, item := range doc.Paths.Map() {
		fiberPath := pathParam.ReplaceAllString(path, ":$1")
		for method := range item.Operations() {
			inContract = append(inContract, method+" "+fiberPath)
		}
	}

	var inApp []string
	for _, r := range newApp(t, nil, Options{}).GetRoutes(true) {
		if r.Method == http.MethodHead { // o Fiber regista HEAD para cada GET
			continue
		}
		inApp = append(inApp, r.Method+" "+r.Path)
	}

	sort.Strings(inContract)
	sort.Strings(inApp)
	if strings.Join(inContract, "\n") != strings.Join(inApp, "\n") {
		t.Errorf("rotas e contrato divergem\ncontrato:\n  %s\napp:\n  %s",
			strings.Join(inContract, "\n  "), strings.Join(inApp, "\n  "))
	}
}

func TestHealthBody(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", http.NoBody)
	res := do(t, newApp(t, errors.New("connection refused"), Options{}), req)

	var report api.HealthReport
	if err := json.Unmarshal(res.body, &report); err != nil {
		t.Fatalf("JSON inválido %q: %v", res.body, err)
	}
	if report.Status != api.Down || report.Checks["mysql"].Status != api.Up || report.Checks["redis"].Status != api.Down {
		t.Errorf("relatório inesperado: %+v", report)
	}
}

func TestNotFoundUsesErrorSchema(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/nao-existe", http.NoBody)
	res := do(t, newApp(t, nil, Options{}), req)

	if res.status != http.StatusNotFound {
		t.Fatalf("status = %d, esperava 404", res.status)
	}
	var body api.Error
	if err := json.Unmarshal(res.body, &body); err != nil || body.Error == "" {
		t.Errorf("corpo não segue o schema Error: %s (%v)", res.body, err)
	}
}

func TestCORS(t *testing.T) {
	const origin = "http://localhost:8090"

	tests := []struct {
		name string
		opts Options
		want string
	}{
		{name: "desativado por omissão", opts: Options{}, want: ""},
		{name: "origem autorizada", opts: Options{CORSAllowedOrigins: []string{origin}}, want: origin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/livez", http.NoBody)
			req.Header.Set("Origin", origin)
			res := do(t, newApp(t, nil, tt.opts), req)

			if got := res.header.Get("Access-Control-Allow-Origin"); got != tt.want {
				t.Errorf("Access-Control-Allow-Origin = %q, esperava %q", got, tt.want)
			}
		})
	}
}
