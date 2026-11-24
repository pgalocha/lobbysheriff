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
		{name: "com rate limit", opts: Options{BaseURL: "https://x.test", APIKey: "k", Timeout: time.Second, RateLimit: 10}},
		{name: "rate limit negativo", opts: Options{BaseURL: "https://x.test", APIKey: "k", Timeout: time.Second, RateLimit: -1}, wantErr: true},
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
		{name: "match stats offset > 200", call: func(ctx context.Context) error {
			_, err := c.GetPlayerMatchStats(ctx, "p1", GameCS2, MatchStatsOptions{Offset: 201})
			return err
		}},
		{name: "match stats sem jogo", call: func(ctx context.Context) error {
			_, err := c.GetPlayerMatchStats(ctx, "p1", "", MatchStatsOptions{})
			return err
		}},
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
		rateLimitReset string
		body           string
		want           error
		wantRetryAfter time.Duration
	}{
		{name: "404", status: http.StatusNotFound, want: ErrNotFound},
		{name: "401", status: http.StatusUnauthorized, want: ErrUnauthorized},
		{name: "403", status: http.StatusForbidden, want: ErrUnauthorized},
		{name: "429 com Retry-After", status: http.StatusTooManyRequests, retryAfter: "30", want: ErrRateLimited, wantRetryAfter: 30 * time.Second},
		{name: "429 só com RateLimit-Reset", status: http.StatusTooManyRequests, rateLimitReset: "3", want: ErrRateLimited, wantRetryAfter: 3 * time.Second},
		{name: "429 com RateLimit-Reset 0", status: http.StatusTooManyRequests, rateLimitReset: "0", want: ErrRateLimited, wantRetryAfter: time.Second},
		{name: "429 sem headers usa a janela de 1s", status: http.StatusTooManyRequests, want: ErrRateLimited, wantRetryAfter: time.Second},
		{name: "404 ignora RateLimit-Reset", status: http.StatusNotFound, rateLimitReset: "3", want: ErrNotFound},
		{name: "500", status: http.StatusInternalServerError, want: ErrUnavailable},
		{name: "503", status: http.StatusServiceUnavailable, retryAfter: "5", want: ErrUnavailable, wantRetryAfter: 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				if tt.rateLimitReset != "" {
					w.Header().Set("RateLimit-Reset", tt.rateLimitReset)
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

func TestRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"player_id":"p1"}`))
	}))
	t.Cleanup(srv.Close)

	// 20 req/s = um pedido a cada 50ms (burst 1): 5 pedidos levam pelo menos
	// 4 intervalos. Só se verifica o mínimo, para o teste não ser instável.
	c, err := New(Options{BaseURL: srv.URL, APIKey: testKey, Timeout: time.Second, RateLimit: 20})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for range 5 {
		if _, err := c.GetPlayer(t.Context(), "p1"); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 4*50*time.Millisecond-10*time.Millisecond {
		t.Errorf("5 pedidos em %s: o rate limit não foi aplicado", elapsed)
	}
}

func TestRateLimitRespectsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"player_id":"p1"}`))
	}))
	t.Cleanup(srv.Close)

	// 1 req/s: o primeiro pedido gasta a vaga; o segundo teria de esperar ~1s,
	// mais do que o prazo do contexto, por isso falha sem chegar ao servidor.
	c, err := New(Options{BaseURL: srv.URL, APIKey: testKey, Timeout: time.Second, RateLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetPlayer(t.Context(), "p1"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.GetPlayer(ctx, "p1"); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("erro = %v, esperava falha à espera do rate limit", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("esperou %s; devia desistir dentro do prazo do contexto", elapsed)
	}
}

func TestGetPlayerMatchStats(t *testing.T) {
	from := time.Unix(1763000000, 0)
	to := time.Unix(1765000000, 0)

	tests := []struct {
		name      string
		opts      MatchStatsOptions
		wantQuery map[string]string
	}{
		{
			name:      "sem filtros",
			opts:      MatchStatsOptions{},
			wantQuery: map[string]string{"from": "", "to": "", "offset": "", "limit": ""},
		},
		{
			// Este endpoint usa milissegundos (o /history usa segundos).
			name:      "intervalo em milissegundos e paginação",
			opts:      MatchStatsOptions{From: from, To: to, Offset: 200, Limit: 100},
			wantQuery: map[string]string{"from": "1763000000000", "to": "1765000000000", "offset": "200", "limit": "100"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := fakeFACEIT(t, map[string]string{"GET /data/v4/players/p1/games/cs2/stats": "player_match_stats_list.json"})

			l, err := c.GetPlayerMatchStats(t.Context(), "p1", GameCS2, tt.opts)
			if err != nil {
				t.Fatalf("GetPlayerMatchStats: %v", err)
			}
			q := reqs.all()[0].URL.Query()
			for k, want := range tt.wantQuery {
				if got := q.Get(k); got != want {
					t.Errorf("query %s = %q, esperava %q", k, got, want)
				}
			}

			if len(l.Items) != 2 {
				t.Fatalf("items = %d, esperava 2", len(l.Items))
			}
			first := l.Items[0]
			if first.MatchID() != "1-00000000-0000-0000-0000-00000000000a" {
				t.Errorf("MatchID = %q", first.MatchID())
			}
			// "Match Finished At" vem como número (ms); StatValues normaliza-o.
			if at, ok := first.FinishedAt(); !ok || !at.Equal(time.Unix(1764002400, 0)) {
				t.Errorf("FinishedAt = %s, %v", at, ok)
			}
			want := map[string]string{"Map": "de_mirage", "Result": "1", "Kills": "24", "ADR": "96.4", "K/D Ratio": "1.6"}
			for k, v := range want {
				if got := first.Stats[k]; got != v {
					t.Errorf("Stats[%q] = %q, esperava %q", k, got, v)
				}
			}
		})
	}
}

func TestPlayerMatchStatsFinishedAtInvalid(t *testing.T) {
	for _, v := range []string{"", "abc", "0", "-5"} {
		s := PlayerMatchStats{Stats: StatValues{"Match Finished At": v}}
		if _, ok := s.FinishedAt(); ok {
			t.Errorf("FinishedAt(%q) devia falhar", v)
		}
	}
}

func TestMatchSummaryTeams(t *testing.T) {
	c, _ := fakeFACEIT(t, map[string]string{"GET /data/v4/players/p1/history": "history.json"})
	h, err := c.GetPlayerHistory(t.Context(), "p1", HistoryOptions{Game: GameCS2})
	if err != nil {
		t.Fatal(err)
	}
	m := h.Items[0]
	if m.CompetitionType != "matchmaking" {
		t.Errorf("CompetitionType = %q", m.CompetitionType)
	}

	const (
		sheriff = "00000000-0000-0000-0000-000000000001"
		deputy  = "00000000-0000-0000-0000-000000000002"
		outlaw  = "00000000-0000-0000-0000-000000000009"
	)
	nicks := func(ps []HistoryPlayer) []string {
		var out []string
		for _, p := range ps {
			out = append(out, p.Nickname)
		}
		return out
	}

	tests := []struct {
		name          string
		playerID      string
		wantFaction   string
		wantWon       bool
		wantOK        bool
		wantTeammates []string
		wantOpponents []string
	}{
		{name: "vencedor", playerID: sheriff, wantFaction: "faction1", wantWon: true, wantOK: true, wantTeammates: []string{"deputy"}, wantOpponents: []string{"outlaw"}},
		{name: "colega de equipa", playerID: deputy, wantFaction: "faction1", wantWon: true, wantOK: true, wantTeammates: []string{"sheriff"}, wantOpponents: []string{"outlaw"}},
		{name: "derrotado", playerID: outlaw, wantFaction: "faction2", wantWon: false, wantOK: true, wantTeammates: nil, wantOpponents: []string{"sheriff", "deputy"}},
		{name: "não jogou", playerID: "x", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			faction, ok := m.FactionOf(tt.playerID)
			if faction != tt.wantFaction || ok != tt.wantOK {
				t.Errorf("FactionOf = %q, %v", faction, ok)
			}
			won, ok := m.Won(tt.playerID)
			if won != tt.wantWon || ok != tt.wantOK {
				t.Errorf("Won = %v, %v", won, ok)
			}
			if got := nicks(m.Teammates(tt.playerID)); strings.Join(got, ",") != strings.Join(tt.wantTeammates, ",") {
				t.Errorf("Teammates = %v, esperava %v", got, tt.wantTeammates)
			}
			if got := nicks(m.Opponents(tt.playerID)); strings.Join(got, ",") != strings.Join(tt.wantOpponents, ",") {
				t.Errorf("Opponents = %v, esperava %v", got, tt.wantOpponents)
			}
		})
	}

	t.Run("sem vencedor", func(t *testing.T) {
		m2 := m
		m2.Results.Winner = ""
		if _, ok := m2.Won(sheriff); ok {
			t.Error("Won devia devolver ok=false sem vencedor")
		}
	})
}
