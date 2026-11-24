package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/config"
	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/faceit"
)

// faceitCommandTimeout limita o subcomando faceit inteiro (vários pedidos).
const faceitCommandTimeout = 30 * time.Second

const faceitUsage = "uso: faceit player <nickname> | faceit last-match <nickname>"

// newFACEITClient cria o cliente FACEIT a partir da configuração, exigindo a key.
func newFACEITClient(cfg config.FACEIT) (*faceit.Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	c, err := faceit.New(faceit.Options{
		BaseURL:   cfg.BaseURL,
		APIKey:    cfg.APIKey,
		Timeout:   cfg.Timeout,
		RateLimit: cfg.RateLimit,
		Retry: faceit.RetryPolicy{
			MaxRetries: cfg.MaxRetries,
			BaseDelay:  faceit.DefaultRetryPolicy.BaseDelay,
			MaxDelay:   faceit.DefaultRetryPolicy.MaxDelay,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cliente FACEIT: %w", err)
	}
	return c, nil
}

// runFACEIT é o subcomando de diagnóstico "faceit": chama a FACEIT real com a
// key configurada e escreve o resultado em JSON. Não passa pela API HTTP, não
// toca no MySQL nem no Redis e não faz parte do contrato.
func runFACEIT(ctx context.Context, cfg config.FACEIT, args []string, out io.Writer) error {
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		return errors.New(faceitUsage)
	}
	c, err := newFACEITClient(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, faceitCommandTimeout)
	defer cancel()

	var result any
	switch args[0] {
	case "player":
		result, err = playerSummary(ctx, c, args[1])
	case "last-match":
		result, err = lastMatchSummary(ctx, c, args[1])
	default:
		return fmt.Errorf("subcomando desconhecido %q (%s)", args[0], faceitUsage)
	}
	if err != nil {
		return err
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// lifetimeKeys são as estatísticas lifetime mostradas (as que vão para
// player_snapshots). Atenção: "K/D Ratio" e "Total Headshots %" são somas na
// FACEIT; as médias estão nas chaves "Average ...".
var lifetimeKeys = []string{"Matches", "Win Rate %", "Average K/D Ratio", "Average Headshots %", "ADR", "Recent Results"}

// matchStatKeys são as estatísticas por partida mostradas (as que vão para
// player_match_stats).
var matchStatKeys = []string{"Result", "Kills", "Deaths", "Assists", "Headshots", "Headshots %", "K/D Ratio", "ADR", "MVPs"}

type playerOutput struct {
	PlayerID   string         `json:"player_id"`
	Nickname   string         `json:"nickname"`
	Country    string         `json:"country"`
	Elo        int            `json:"elo"`
	SkillLevel int            `json:"skill_level"`
	Lifetime   map[string]any `json:"lifetime"`
}

func playerSummary(ctx context.Context, c *faceit.Client, nickname string) (playerOutput, error) {
	p, err := lookupCS2Player(ctx, c, nickname)
	if err != nil {
		return playerOutput{}, err
	}
	cs2, _ := p.CS2()

	stats, err := c.GetPlayerStats(ctx, p.PlayerID, faceit.GameCS2)
	if err != nil {
		return playerOutput{}, fmt.Errorf("estatísticas de %q: %w", nickname, err)
	}
	lifetime := make(map[string]any, len(lifetimeKeys))
	for _, k := range lifetimeKeys {
		if v, ok := stats.Lifetime[k]; ok {
			lifetime[k] = v
		}
	}

	return playerOutput{
		PlayerID:   p.PlayerID,
		Nickname:   p.Nickname,
		Country:    p.Country,
		Elo:        cs2.FaceitElo,
		SkillLevel: cs2.SkillLevel,
		Lifetime:   lifetime,
	}, nil
}

type lastMatchOutput struct {
	MatchID     string      `json:"match_id"`
	URL         string      `json:"url"`
	Competition string      `json:"competition"`
	FinishedAt  time.Time   `json:"finished_at"`
	Maps        []mapOutput `json:"maps"`
}

type mapOutput struct {
	Map   string            `json:"map"`
	Score string            `json:"score"`
	Stats map[string]string `json:"player_stats"`
}

func lastMatchSummary(ctx context.Context, c *faceit.Client, nickname string) (lastMatchOutput, error) {
	p, err := lookupCS2Player(ctx, c, nickname)
	if err != nil {
		return lastMatchOutput{}, err
	}

	h, err := c.GetPlayerHistory(ctx, p.PlayerID, faceit.HistoryOptions{Game: faceit.GameCS2, Limit: 1})
	if err != nil {
		return lastMatchOutput{}, fmt.Errorf("histórico de %q: %w", nickname, err)
	}
	if len(h.Items) == 0 {
		return lastMatchOutput{}, fmt.Errorf("%q não tem partidas de CS2", nickname)
	}
	m := h.Items[0]

	stats, err := c.GetMatchStats(ctx, m.MatchID)
	if err != nil {
		return lastMatchOutput{}, fmt.Errorf("estatísticas da partida %s: %w", m.MatchID, err)
	}

	out := lastMatchOutput{
		MatchID:     m.MatchID,
		URL:         strings.ReplaceAll(m.FaceitURL, "{lang}", "en"),
		Competition: m.CompetitionName,
		FinishedAt:  time.Unix(m.FinishedAt, 0).UTC(),
	}
	// Uma partida pode ter vários mapas (BO3); mostra-se cada um.
	for _, r := range stats.Rounds {
		mo := mapOutput{Map: r.RoundStats["Map"], Score: r.RoundStats["Score"]}
		for _, team := range r.Teams {
			for _, pl := range team.Players {
				if pl.PlayerID != p.PlayerID {
					continue
				}
				mo.Stats = make(map[string]string, len(matchStatKeys))
				for _, k := range matchStatKeys {
					if v, ok := pl.PlayerStats[k]; ok {
						mo.Stats[k] = v
					}
				}
			}
		}
		out.Maps = append(out.Maps, mo)
	}
	return out, nil
}

// lookupCS2Player procura o jogador pelo nickname e exige que jogue CS2.
func lookupCS2Player(ctx context.Context, c *faceit.Client, nickname string) (faceit.Player, error) {
	p, err := c.GetPlayerByNickname(ctx, nickname)
	if errors.Is(err, faceit.ErrNotFound) {
		return faceit.Player{}, fmt.Errorf("jogador %q não existe na FACEIT", nickname)
	}
	if err != nil {
		return faceit.Player{}, fmt.Errorf("procurar %q: %w", nickname, err)
	}
	if _, ok := p.CS2(); !ok {
		return faceit.Player{}, fmt.Errorf("%q não tem perfil de CS2 na FACEIT", nickname)
	}
	return p, nil
}
