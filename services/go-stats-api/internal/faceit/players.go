package faceit

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"
)

// GetPlayer devolve um jogador pelo ID FACEIT.
func (c *Client) GetPlayer(ctx context.Context, playerID string) (Player, error) {
	id, err := segment("player_id", playerID)
	if err != nil {
		return Player{}, err
	}
	var p Player
	err = c.get(ctx, "/players/"+id, nil, &p)
	return p, err
}

// GetPlayerByNickname procura um jogador pelo nickname FACEIT.
func (c *Client) GetPlayerByNickname(ctx context.Context, nickname string) (Player, error) {
	if nickname == "" {
		return Player{}, errors.New("faceit: nickname vazio")
	}
	var p Player
	err := c.get(ctx, "/players", url.Values{"nickname": {nickname}}, &p)
	return p, err
}

// GetPlayerStats devolve as estatísticas agregadas de um jogador num jogo
// (normalmente GameCS2).
func (c *Client) GetPlayerStats(ctx context.Context, playerID, gameID string) (PlayerStats, error) {
	id, err := segment("player_id", playerID)
	if err != nil {
		return PlayerStats{}, err
	}
	game, err := segment("game_id", gameID)
	if err != nil {
		return PlayerStats{}, err
	}
	var s PlayerStats
	err = c.get(ctx, "/players/"+id+"/stats/"+game, nil, &s)
	return s, err
}

// HistoryOptions filtra e pagina o histórico de partidas.
type HistoryOptions struct {
	// Game, ex.: GameCS2. Obrigatório.
	Game string
	// From/To limitam o intervalo de datas; zero = sem limite.
	From, To time.Time
	// Offset (máx. 1000) e Limit (máx. 100; zero = 20, o valor por omissão da
	// FACEIT) paginam.
	Offset, Limit int
}

// GetPlayerHistory devolve uma página do histórico de partidas de um jogador.
func (c *Client) GetPlayerHistory(ctx context.Context, playerID string, opts HistoryOptions) (MatchHistory, error) {
	id, err := segment("player_id", playerID)
	if err != nil {
		return MatchHistory{}, err
	}
	if opts.Game == "" {
		return MatchHistory{}, errors.New("faceit: game vazio")
	}
	if opts.Offset < 0 || opts.Offset > 1000 || opts.Limit < 0 || opts.Limit > 100 {
		return MatchHistory{}, errors.New("faceit: paginação inválida (0 <= offset <= 1000, 0 <= limit <= 100)")
	}

	q := url.Values{"game": {opts.Game}}
	if !opts.From.IsZero() {
		q.Set("from", strconv.FormatInt(opts.From.Unix(), 10))
	}
	if !opts.To.IsZero() {
		q.Set("to", strconv.FormatInt(opts.To.Unix(), 10))
	}
	if opts.Offset > 0 {
		q.Set("offset", strconv.Itoa(opts.Offset))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}

	var h MatchHistory
	err = c.get(ctx, "/players/"+id+"/history", q, &h)
	return h, err
}

// MatchStatsOptions filtra e pagina as estatísticas por partida.
type MatchStatsOptions struct {
	// From/To limitam pela data de fim da partida; zero = sem limite.
	// (Este endpoint usa milissegundos, ao contrário do /history; a conversão
	// é feita aqui.)
	From, To time.Time
	// Offset (máx. 200) e Limit (máx. 100; zero = 20, o valor por omissão da
	// FACEIT) paginam.
	Offset, Limit int
}

// GetPlayerMatchStats devolve as estatísticas do jogador em cada partida
// recente, até 100 por pedido. Evita um pedido /matches/{id}/stats por
// partida quando só interessam as estatísticas deste jogador.
func (c *Client) GetPlayerMatchStats(ctx context.Context, playerID, gameID string, opts MatchStatsOptions) (PlayerMatchStatsList, error) {
	id, err := segment("player_id", playerID)
	if err != nil {
		return PlayerMatchStatsList{}, err
	}
	game, err := segment("game_id", gameID)
	if err != nil {
		return PlayerMatchStatsList{}, err
	}
	if opts.Offset < 0 || opts.Offset > 200 || opts.Limit < 0 || opts.Limit > 100 {
		return PlayerMatchStatsList{}, errors.New("faceit: paginação inválida (0 <= offset <= 200, 0 <= limit <= 100)")
	}

	q := url.Values{}
	if !opts.From.IsZero() {
		q.Set("from", strconv.FormatInt(opts.From.UnixMilli(), 10))
	}
	if !opts.To.IsZero() {
		q.Set("to", strconv.FormatInt(opts.To.UnixMilli(), 10))
	}
	if opts.Offset > 0 {
		q.Set("offset", strconv.Itoa(opts.Offset))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}

	var l PlayerMatchStatsList
	err = c.get(ctx, "/players/"+id+"/games/"+game+"/stats", q, &l)
	return l, err
}
