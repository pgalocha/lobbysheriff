package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/config"
)

// Fixtures do pacote faceit (estrutura real da FACEIT, dados sintéticos).
var faceitFixtures = os.DirFS("../../internal/faceit/testdata")

const fixturePlayerID = "00000000-0000-0000-0000-000000000001"

// fakeFACEITConfig arranca uma FACEIT falsa e devolve a config para a usar.
// routes: "caminho" → fixture; nickname desconhecido → 404.
func fakeFACEITConfig(t *testing.T, routes map[string]string) config.FACEIT {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		path := r.URL.Path
		if path == "/players" {
			path += "?nickname=" + r.URL.Query().Get("nickname")
		}
		fixture, ok := routes[path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, err := fs.ReadFile(faceitFixtures, fixture)
		if err != nil {
			t.Errorf("ler fixture %s: %v", fixture, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return config.FACEIT{APIKey: "test-key", BaseURL: srv.URL, Timeout: time.Second}
}

var allRoutes = map[string]string{
	"/players?nickname=sheriff":                             "player.json",
	"/players/" + fixturePlayerID + "/stats/cs2":            "player_stats.json",
	"/players/" + fixturePlayerID + "/history":              "history.json",
	"/matches/1-00000000-0000-0000-0000-00000000000a/stats": "match_stats.json",
}

func TestRunFACEITPlayer(t *testing.T) {
	cfg := fakeFACEITConfig(t, allRoutes)
	var out bytes.Buffer

	if err := runFACEIT(t.Context(), cfg, []string{"player", "sheriff"}, &out); err != nil {
		t.Fatalf("runFACEIT: %v", err)
	}
	var got playerOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("saída não é JSON: %v\n%s", err, out.String())
	}
	if got.PlayerID != fixturePlayerID || got.Elo != 1780 || got.SkillLevel != 8 {
		t.Errorf("player = %+v", got)
	}
	want := map[string]any{"Matches": "412", "Win Rate %": "53", "Average K/D Ratio": "1.12", "Average Headshots %": "48", "ADR": "79.3"}
	for k, v := range want {
		if got.Lifetime[k] != v {
			t.Errorf("lifetime[%q] = %v, esperava %v", k, got.Lifetime[k], v)
		}
	}
	if _, ok := got.Lifetime["K/D Ratio"]; ok {
		t.Error(`"K/D Ratio" do lifetime é uma soma e não deve ser mostrado`)
	}
}

func TestRunFACEITLastMatch(t *testing.T) {
	cfg := fakeFACEITConfig(t, allRoutes)
	var out bytes.Buffer

	if err := runFACEIT(t.Context(), cfg, []string{"last-match", "sheriff"}, &out); err != nil {
		t.Fatalf("runFACEIT: %v", err)
	}
	var got lastMatchOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("saída não é JSON: %v\n%s", err, out.String())
	}
	if got.MatchID != "1-00000000-0000-0000-0000-00000000000a" || strings.Contains(got.URL, "{lang}") {
		t.Errorf("match = %+v", got)
	}
	if !got.FinishedAt.Equal(time.Unix(1764002400, 0)) {
		t.Errorf("finished_at = %s", got.FinishedAt)
	}
	if len(got.Maps) != 1 || got.Maps[0].Map != "de_mirage" || got.Maps[0].Score != "13 / 9" {
		t.Fatalf("maps = %+v", got.Maps)
	}
	if s := got.Maps[0].Stats; s["Kills"] != "24" || s["Result"] != "1" || s["ADR"] != "96.4" {
		t.Errorf("player_stats = %v", s)
	}
}

func TestRunFACEITErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		apiKey  string
		routes  map[string]string
		wantErr string
	}{
		{name: "sem argumentos", args: nil, apiKey: "test-key", wantErr: "uso:"},
		{name: "nickname vazio", args: []string{"player", " "}, apiKey: "test-key", wantErr: "uso:"},
		{name: "subcomando desconhecido", args: []string{"team", "sheriff"}, apiKey: "test-key", wantErr: "desconhecido"},
		{name: "sem key", args: []string{"player", "sheriff"}, apiKey: "", wantErr: "FACEIT_API_KEY"},
		{name: "jogador inexistente", args: []string{"player", "ninguem"}, apiKey: "test-key", routes: allRoutes, wantErr: "não existe"},
		{name: "sem partidas", args: []string{"last-match", "sheriff"}, apiKey: "test-key", routes: map[string]string{
			"/players?nickname=sheriff":                "player.json",
			"/players/" + fixturePlayerID + "/history": "empty_history.json",
		}, wantErr: "não tem partidas"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := fakeFACEITConfig(t, tt.routes)
			cfg.APIKey = tt.apiKey
			var out bytes.Buffer

			err := runFACEIT(t.Context(), cfg, tt.args, &out)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("erro = %v, esperava conter %q", err, tt.wantErr)
			}
			if out.Len() != 0 {
				t.Errorf("não devia escrever nada no stdout em caso de erro: %s", out.String())
			}
		})
	}
}
