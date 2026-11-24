// Package faceit é o cliente da FACEIT Data API v4 (https://docs.faceit.com/).
//
// Só cobre os endpoints de que o go-stats-api precisa e só descodifica os
// campos que usamos. A API key vai no header Authorization e nunca aparece em
// erros nem em logs.
package faceit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GameCS2 é o identificador do Counter-Strike 2 na FACEIT.
const GameCS2 = "cs2"

// maxBodyBytes limita o tamanho das respostas lidas (proteção contra respostas
// anómalas; as maiores, de estatísticas de partidas, ficam muito abaixo).
const maxBodyBytes = 10 << 20

// Options configura o Client.
type Options struct {
	// BaseURL da API, ex.: https://open.faceit.com/data/v4. Nos testes aponta
	// para um httptest.Server.
	BaseURL string
	// APIKey server-side da FACEIT.
	APIKey string
	// Timeout de cada pedido. Ignorado se HTTPClient for indicado.
	Timeout time.Duration
	// HTTPClient opcional (ex.: para testes ou transports instrumentados).
	HTTPClient *http.Client
}

// Client chama a FACEIT Data API v4. É seguro para uso concorrente.
type Client struct {
	baseURL *url.URL
	apiKey  string
	http    *http.Client
}

// New valida as opções e cria o Client.
func New(opts Options) (*Client, error) {
	if opts.APIKey == "" {
		return nil, errors.New("faceit: API key em falta")
	}
	u, err := url.Parse(strings.TrimRight(opts.BaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("faceit: base URL inválida: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("faceit: base URL inválida %q: tem de ser http(s)://host[/caminho]", opts.BaseURL)
	}

	hc := opts.HTTPClient
	if hc == nil {
		if opts.Timeout <= 0 {
			return nil, errors.New("faceit: timeout tem de ser positivo")
		}
		hc = &http.Client{Timeout: opts.Timeout}
	}

	return &Client{baseURL: u, apiKey: opts.APIKey, http: hc}, nil
}

// get faz GET a path (já com os segmentos escapados) e descodifica o JSON em out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	// Concatenação de strings (e não u.Path): os segmentos já vêm escapados
	// e não podem ser escapados duas vezes nem normalizados ("../").
	target := c.baseURL.String() + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return fmt.Errorf("faceit: GET %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "lobbysheriff-go-stats-api")

	resp, err := c.http.Do(req)
	if err != nil {
		// O erro do net/http inclui o URL, que não tem a key (vai no header).
		return fmt.Errorf("faceit: GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Esvaziar o corpo permite reutilizar a ligação.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
		return &APIError{
			Method:     http.MethodGet,
			Path:       path,
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		}
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(out); err != nil {
		return fmt.Errorf("faceit: GET %s: resposta inválida: %w", path, err)
	}
	return nil
}

// segment escapa um identificador para usar no caminho do URL e rejeita vazios.
func segment(name, v string) (string, error) {
	if strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("faceit: %s vazio", name)
	}
	return url.PathEscape(v), nil
}
