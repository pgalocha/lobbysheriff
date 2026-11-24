package faceit

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Key falsa: os testes nunca usam a key real (ver regras do projeto).
const testKey = "test-key"

// fixtures têm a estrutura de respostas reais da FACEIT (validada em
// 2026-11-24), com IDs, nicknames e valores sintéticos.
var fixtures = os.DirFS("testdata")

// recorder guarda os pedidos recebidos pelo servidor falso. O handler corre
// noutra goroutine, por isso o acesso é protegido (go test -race).
type recorder struct {
	mu   sync.Mutex
	reqs []*http.Request
}

func (r *recorder) add(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, req)
}

func (r *recorder) all() []*http.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*http.Request(nil), r.reqs...)
}

// fakeFACEIT simula a FACEIT: valida o header de autenticação e responde
// com o fixture de testdata/ associado a "METHOD caminho".
func fakeFACEIT(t *testing.T, routes map[string]string) (*Client, *recorder) {
	t.Helper()
	rec := &recorder{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r.Clone(context.Background()))
		if got := r.Header.Get("Authorization"); got != "Bearer "+testKey {
			http.Error(w, `{"errors":[{"message":"unauthorized"}]}`, http.StatusUnauthorized)
			return
		}
		fixture, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			http.Error(w, `{"errors":[{"message":"not found"}]}`, http.StatusNotFound)
			return
		}
		body, err := fs.ReadFile(fixtures, fixture)
		if err != nil {
			t.Errorf("ler fixture %s: %v", fixture, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c, err := New(Options{BaseURL: srv.URL + "/data/v4", APIKey: testKey, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, rec
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{name: "válido", opts: Options{BaseURL: "https://open.faceit.com/data/v4", APIKey: "k", Timeout: time.Second}},
		{name: "barra final", opts: Options{BaseURL: "https://open.faceit.com/data/v4/", APIKey: "k", Timeout: time.Second}},
		{name: "HTTPClient dispensa timeout", opts: Options{BaseURL: "https://x.test", APIKey: "k", HTTPClient: &http.Client{}}},
		{name: "sem key", opts: Options{BaseURL: "https://x.test", Timeout: time.Second}, wantErr: true},
		{name: "sem esquema", opts: Options{BaseURL: "open.faceit.com", APIKey: "k", Timeout: time.Second}, wantErr: true},
		{name: "esquema inválido", opts: Options{BaseURL: "ftp://x.test", APIKey: "k", Timeout: time.Second}, wantErr: true},
		{name: "timeout zero", opts: Options{BaseURL: "https://x.test", APIKey: "k"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("New() erro = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), tt.opts.APIKey) && tt.opts.APIKey != "" {
				t.Errorf("o erro expõe a API key: %v", err)
			}
		})
	}
}

func TestGetPlayer(t *testing.T) {
	c, reqs := fakeFACEIT(t, map[string]string{
		"GET /data/v4/players/00000000-0000-0000-0000-000000000001": "player.json",
	})

	p, err := c.GetPlayer(t.Context(), "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("GetPlayer: %v", err)
	}
	if p.Nickname != "sheriff" || p.Country != "pt" {
		t.Errorf("player = %+v", p)
	}
	cs2, ok := p.CS2()
	if !ok || cs2.FaceitElo != 1780 || cs2.SkillLevel != 8 {
		t.Errorf("CS2() = %+v, %v; esperava elo 1780, nível 8", cs2, ok)
	}

	r := reqs.all()[0]
	if got := r.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
	if got := r.Header.Get("User-Agent"); got == "" {
		t.Error("User-Agent em falta")
	}
}

func TestGetPlayerByNickname(t *testing.T) {
	c, reqs := fakeFACEIT(t, map[string]string{"GET /data/v4/players": "player.json"})

	p, err := c.GetPlayerByNickname(t.Context(), "sheriff & co")
	if err != nil {
		t.Fatalf("GetPlayerByNickname: %v", err)
	}
	if p.PlayerID != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("PlayerID = %q", p.PlayerID)
	}
	if got := reqs.all()[0].URL.Query().Get("nickname"); got != "sheriff & co" {
		t.Errorf("nickname na query = %q", got)
	}
}

func TestGetPlayerStats(t *testing.T) {
	c, _ := fakeFACEIT(t, map[string]string{
		"GET /data/v4/players/p1/stats/cs2": "player_stats.json",
	})

	s, err := c.GetPlayerStats(t.Context(), "p1", GameCS2)
	if err != nil {
		t.Fatalf("GetPlayerStats: %v", err)
	}
	// Atenção: no lifetime, "K/D Ratio" e "Total Headshots %" são somas;
	// as médias estão em "Average K/D Ratio" e "Average Headshots %".
	want := map[string]any{"Matches": "412", "Win Rate %": "53", "Average K/D Ratio": "1.12", "Average Headshots %": "48", "ADR": "79.3"}
	if s.GameID != "cs2" {
		t.Errorf("GameID = %q", s.GameID)
	}
	for k, v := range want {
		if got := s.Lifetime[k]; got != v {
			t.Errorf("Lifetime[%q] = %v, esperava %v", k, got, v)
		}
	}
}

func TestGetPlayerHistory(t *testing.T) {
	from := time.Unix(1763000000, 0)
	to := time.Unix(1765000000, 0)

	tests := []struct {
		name      string
		opts      HistoryOptions
		wantQuery map[string]string
	}{
		{
			name:      "só o jogo",
			opts:      HistoryOptions{Game: GameCS2},
			wantQuery: map[string]string{"game": "cs2", "from": "", "to": "", "offset": "", "limit": ""},
		},
		{
			name:      "intervalo e paginação",
			opts:      HistoryOptions{Game: GameCS2, From: from, To: to, Offset: 20, Limit: 100},
			wantQuery: map[string]string{"game": "cs2", "from": "1763000000", "to": "1765000000", "offset": "20", "limit": "100"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := fakeFACEIT(t, map[string]string{"GET /data/v4/players/p1/history": "history.json"})

			h, err := c.GetPlayerHistory(t.Context(), "p1", tt.opts)
			if err != nil {
				t.Fatalf("GetPlayerHistory: %v", err)
			}
			if len(h.Items) != 1 || h.Items[0].Status != "finished" || h.Items[0].FinishedAt != 1764002400 {
				t.Errorf("history = %+v", h)
			}
			if r := h.Items[0].Results; r.Winner != "faction1" || r.Score["faction1"] != 13 {
				t.Errorf("results = %+v", r)
			}
			q := reqs.all()[0].URL.Query()
			for k, want := range tt.wantQuery {
				if got := q.Get(k); got != want {
					t.Errorf("query %s = %q, esperava %q", k, got, want)
				}
			}
		})
	}
}

func TestGetMatchStats(t *testing.T) {
	c, _ := fakeFACEIT(t, map[string]string{
		"GET /data/v4/matches/1-00000000-0000-0000-0000-00000000000a/stats": "match_stats.json",
	})

	s, err := c.GetMatchStats(t.Context(), "1-00000000-0000-0000-0000-00000000000a")
	if err != nil {
		t.Fatalf("GetMatchStats: %v", err)
	}
	if len(s.Rounds) != 1 || s.Rounds[0].RoundStats["Map"] != "de_mirage" {
		t.Fatalf("rounds = %+v", s.Rounds)
	}
	pl := s.Rounds[0].Teams[0].Players[0]
	if pl.Nickname != "sheriff" {
		t.Errorf("player = %+v", pl)
	}
	// As chaves usadas para preencher player_match_stats.
	want := map[string]string{
		"Kills": "24", "Deaths": "15", "Assists": "4", "Headshots": "12", "MVPs": "5",
		"K/D Ratio": "1.6", "ADR": "96.4", "Headshots %": "50", "Result": "1",
	}
	for k, v := range want {
		if got := pl.PlayerStats[k]; got != v {
			t.Errorf("PlayerStats[%q] = %q, esperava %q", k, got, v)
		}
	}
}

func TestStatValuesUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    StatValues
		wantErr bool
	}{
		{name: "strings", in: `{"Map":"de_mirage"}`, want: StatValues{"Map": "de_mirage"}},
		{name: "escapes JSON", in: `{"Team":"a\u00e7\"b"}`, want: StatValues{"Team": "aç\"b"}},
		{name: "número e booleano", in: `{"Kills":24,"Win":false}`, want: StatValues{"Kills": "24", "Win": "false"}},
		{name: "null é ignorado", in: `{"ADR":null,"Kills":"3"}`, want: StatValues{"Kills": "3"}},
		{name: "objeto fica em JSON", in: `{"x":{"a":1}}`, want: StatValues{"x": `{"a":1}`}},
		{name: "null no topo", in: `null`, want: nil},
		{name: "não é objeto", in: `[1,2]`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got StatValues
			err := got.UnmarshalJSON([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Fatalf("erro = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %q, esperava %q", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("[%q] = %q, esperava %q", k, got[k], v)
				}
			}
		})
	}
}

func TestInvalidArguments(t *testing.T) {
	c, reqs := fakeFACEIT(t, nil)

	tests := []struct {
		name string
		call func(context.Context) error
	}{
		{name: "player_id vazio", call: func(ctx context.Context) error { _, err := c.GetPlayer(ctx, " "); return err }},
		{name: "nickname vazio", call: func(ctx context.Context) error { _, err := c.GetPlayerByNickname(ctx, ""); return err }},
		{name: "game_id vazio", call: func(ctx context.Context) error { _, err := c.GetPlayerStats(ctx, "p1", ""); return err }},
		{name: "history sem jogo", call: func(ctx context.Context) error {
			_, err := c.GetPlayerHistory(ctx, "p1", HistoryOptions{})
			return err
		}},
		{name: "history offset > 1000", call: func(ctx context.Context) error {
			_, err := c.GetPlayerHistory(ctx, "p1", HistoryOptions{Game: GameCS2, Offset: 1001})
			return err
		}},
		{name: "history limit > 100", call: func(ctx context.Context) error {
			_, err := c.GetPlayerHistory(ctx, "p1", HistoryOptions{Game: GameCS2, Limit: 101})
			return err
		}},
		{name: "match_id vazio", call: func(ctx context.Context) error { _, err := c.GetMatchStats(ctx, ""); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(t.Context()); err == nil {
				t.Fatal("esperava erro")
			}
		})
	}
	if len(reqs.all()) != 0 {
		t.Errorf("argumentos inválidos não devem gerar pedidos; houve %d", len(reqs.all()))
	}
}

func TestPathEscaping(t *testing.T) {
	c, reqs := fakeFACEIT(t, nil)

	_, _ = c.GetPlayer(t.Context(), "../matches/x")
	if got := reqs.all()[0].URL.EscapedPath(); got != "/data/v4/players/..%2Fmatches%2Fx" {
		t.Errorf("caminho = %q, esperava o ID escapado", got)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		retryAfter     string
		body           string
		want           error
		wantRetryAfter time.Duration
	}{
		{name: "404", status: http.StatusNotFound, want: ErrNotFound},
		{name: "401", status: http.StatusUnauthorized, want: ErrUnauthorized},
		{name: "403", status: http.StatusForbidden, want: ErrUnauthorized},
		{name: "429 com Retry-After", status: http.StatusTooManyRequests, retryAfter: "30", want: ErrRateLimited, wantRetryAfter: 30 * time.Second},
		{name: "429 sem Retry-After", status: http.StatusTooManyRequests, want: ErrRateLimited},
		{name: "500", status: http.StatusInternalServerError, want: ErrUnavailable},
		{name: "503", status: http.StatusServiceUnavailable, retryAfter: "5", want: ErrUnavailable, wantRetryAfter: 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"errors":[{"message":"x"}]}`))
			}))
			t.Cleanup(srv.Close)
			c, err := New(Options{BaseURL: srv.URL, APIKey: testKey, Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}

			_, err = c.GetPlayer(t.Context(), "p1")
			if !errors.Is(err, tt.want) {
				t.Fatalf("erro = %v, esperava %v", err, tt.want)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("esperava *APIError, obtive %T", err)
			}
			if apiErr.StatusCode != tt.status || apiErr.RetryAfter != tt.wantRetryAfter {
				t.Errorf("APIError = %+v", apiErr)
			}
			if strings.Contains(err.Error(), testKey) {
				t.Errorf("o erro expõe a API key: %v", err)
			}
		})
	}
}

func TestInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"player_id":`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{BaseURL: srv.URL, APIKey: testKey, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.GetPlayer(t.Context(), "p1"); err == nil || !strings.Contains(err.Error(), "resposta inválida") {
		t.Fatalf("erro = %v, esperava resposta inválida", err)
	}
}

func TestContextCanceled(t *testing.T) {
	c, _ := fakeFACEIT(t, map[string]string{"GET /data/v4/players/p1": "player.json"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := c.GetPlayer(ctx, "p1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, esperava context.Canceled", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		in   string
		want time.Duration
	}{
		{name: "vazio", in: "", want: 0},
		{name: "segundos", in: "120", want: 2 * time.Minute},
		{name: "negativo", in: "-1", want: 0},
		{name: "data HTTP futura", in: now.Add(time.Minute).Format(http.TimeFormat), want: time.Minute},
		{name: "data HTTP passada", in: now.Add(-time.Minute).Format(http.TimeFormat), want: 0},
		{name: "lixo", in: "amanhã", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRetryAfter(tt.in, now); got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %s, esperava %s", tt.in, got, tt.want)
			}
		})
	}
}
