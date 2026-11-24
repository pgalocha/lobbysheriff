package faceit

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"
)

// Player é um jogador FACEIT (GET /players e /players/{player_id}).
type Player struct {
	PlayerID  string                `json:"player_id"`
	Nickname  string                `json:"nickname"`
	Avatar    string                `json:"avatar"`
	Country   string                `json:"country"`
	FaceitURL string                `json:"faceit_url"`
	Games     map[string]PlayerGame `json:"games"`
}

// CS2 devolve os dados do jogador no CS2, se os tiver.
func (p Player) CS2() (PlayerGame, bool) {
	g, ok := p.Games[GameCS2]
	return g, ok
}

// PlayerGame são os dados do jogador num jogo.
type PlayerGame struct {
	Region         string `json:"region"`
	GamePlayerID   string `json:"game_player_id"`
	GamePlayerName string `json:"game_player_name"`
	SkillLevel     int    `json:"skill_level"`
	FaceitElo      int    `json:"faceit_elo"`
}

// PlayerStats são as estatísticas agregadas de um jogador num jogo
// (GET /players/{player_id}/stats/{game_id}).
//
// A FACEIT devolve os valores com nomes legíveis ("Average K/D Ratio") e, na
// maioria, como strings; por isso o Lifetime fica em bruto e a interpretação
// é feita por quem o consome.
type PlayerStats struct {
	PlayerID string         `json:"player_id"`
	GameID   string         `json:"game_id"`
	Lifetime map[string]any `json:"lifetime"`
}

// MatchHistory é uma página do histórico de partidas
// (GET /players/{player_id}/history).
type MatchHistory struct {
	Items []MatchSummary `json:"items"`
	Start int            `json:"start"`
	End   int            `json:"end"`
}

// MatchSummary é uma partida do histórico. Datas em segundos Unix.
type MatchSummary struct {
	MatchID         string `json:"match_id"`
	GameID          string `json:"game_id"`
	GameMode        string `json:"game_mode"`
	CompetitionName string `json:"competition_name"`
	// CompetitionType: "matchmaking" (fila normal), "hub", "championship", ...
	CompetitionType string `json:"competition_type"`
	Status          string `json:"status"`
	StartedAt       int64  `json:"started_at"`
	FinishedAt      int64  `json:"finished_at"`
	FaceitURL       string `json:"faceit_url"`
	// Results: vencedor ("faction1"/"faction2") e resultado por fação.
	Results MatchResult `json:"results"`
	// Teams: as duas fações ("faction1", "faction2") com os jogadores.
	Teams map[string]HistoryTeam `json:"teams"`
}

// HistoryTeam é uma fação numa partida do histórico.
type HistoryTeam struct {
	TeamID   string          `json:"team_id"`
	Nickname string          `json:"nickname"`
	Players  []HistoryPlayer `json:"players"`
}

// HistoryPlayer é um jogador de uma fação no histórico.
type HistoryPlayer struct {
	PlayerID   string `json:"player_id"`
	Nickname   string `json:"nickname"`
	Avatar     string `json:"avatar"`
	SkillLevel int    `json:"skill_level"`
}

// FactionOf devolve a fação ("faction1"/"faction2") em que playerID jogou.
func (m MatchSummary) FactionOf(playerID string) (string, bool) {
	for faction, team := range m.Teams {
		for _, p := range team.Players {
			if p.PlayerID == playerID {
				return faction, true
			}
		}
	}
	return "", false
}

// Won diz se playerID ganhou; ok = false se não jogou ou não há vencedor.
func (m MatchSummary) Won(playerID string) (won, ok bool) {
	faction, found := m.FactionOf(playerID)
	if !found || m.Results.Winner == "" {
		return false, false
	}
	return faction == m.Results.Winner, true
}

// Teammates devolve os colegas de equipa de playerID (sem ele próprio).
func (m MatchSummary) Teammates(playerID string) []HistoryPlayer {
	faction, ok := m.FactionOf(playerID)
	if !ok {
		return nil
	}
	var out []HistoryPlayer
	for _, p := range m.Teams[faction].Players {
		if p.PlayerID != playerID {
			out = append(out, p)
		}
	}
	return out
}

// Opponents devolve os jogadores da fação adversária de playerID.
func (m MatchSummary) Opponents(playerID string) []HistoryPlayer {
	faction, ok := m.FactionOf(playerID)
	if !ok {
		return nil
	}
	var out []HistoryPlayer
	for f, team := range m.Teams {
		if f != faction {
			out = append(out, team.Players...)
		}
	}
	return out
}

// MatchResult é o resultado de uma partida do histórico.
type MatchResult struct {
	Winner string           `json:"winner"`
	Score  map[string]int64 `json:"score"`
}

// MatchStats são as estatísticas de uma partida (GET /matches/{match_id}/stats).
// Uma partida pode ter vários mapas (rounds), por ex. num BO3.
type MatchStats struct {
	Rounds []MatchRound `json:"rounds"`
}

// MatchRound é um mapa jogado. As chaves de RoundStats/TeamStats/PlayerStats
// são as da FACEIT ("Map", "Score", "Kills", "K/D Ratio", ...).
type MatchRound struct {
	MatchID    string      `json:"match_id"`
	RoundStats StatValues  `json:"round_stats"`
	Teams      []MatchTeam `json:"teams"`
}

// MatchTeam é uma equipa num mapa.
type MatchTeam struct {
	TeamID    string        `json:"team_id"`
	TeamStats StatValues    `json:"team_stats"`
	Players   []MatchPlayer `json:"players"`
}

// MatchPlayer são as estatísticas de um jogador num mapa.
type MatchPlayer struct {
	PlayerID    string     `json:"player_id"`
	Nickname    string     `json:"nickname"`
	PlayerStats StatValues `json:"player_stats"`
}

// StatValues são estatísticas chave → valor, normalizadas para string.
//
// O contrato da FACEIT não fixa o tipo dos valores (additionalProperties: {});
// na prática vêm como strings ("24", "1.6"), mas um número ou booleano não
// deve fazer falhar a descodificação da partida inteira. Números e booleanos
// ficam no seu texto JSON; null é ignorado; objetos e arrays ficam em JSON.
type StatValues map[string]string

// UnmarshalJSON implementa a normalização descrita em StatValues.
func (s *StatValues) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		*s = nil
		return nil
	}
	out := make(StatValues, len(raw))
	for k, v := range raw {
		v = bytes.TrimSpace(v)
		switch {
		case bytes.Equal(v, []byte("null")):
			continue
		case len(v) > 0 && v[0] == '"':
			var str string
			if err := json.Unmarshal(v, &str); err != nil {
				return err
			}
			out[k] = str
		default:
			out[k] = string(v)
		}
	}
	*s = out
	return nil
}

// PlayerMatchStatsList é uma página de estatísticas do jogador por partida
// (GET /players/{player_id}/games/{game_id}/stats).
type PlayerMatchStatsList struct {
	Items []PlayerMatchStats `json:"items"`
	Start int                `json:"start"`
	End   int                `json:"end"`
}

// PlayerMatchStats são as estatísticas de um jogador num mapa de uma partida.
// Numa partida com vários mapas (BO3) há um item por mapa ("Match Round").
//
// As chaves são as da FACEIT: "Match Id", "Map", "Score", "Result" ("1"/"0"),
// "Kills", "Deaths", "Assists", "Headshots", "Headshots %", "K/D Ratio",
// "ADR", "MVPs", "Damage", "Match Finished At" (ms desde a epoch), ...
type PlayerMatchStats struct {
	Stats StatValues `json:"stats"`
}

// MatchID devolve o ID da partida.
func (s PlayerMatchStats) MatchID() string { return s.Stats["Match Id"] }

// FinishedAt devolve o fim da partida; ok = false se o valor faltar ou for inválido.
func (s PlayerMatchStats) FinishedAt() (t time.Time, ok bool) {
	ms, err := strconv.ParseInt(s.Stats["Match Finished At"], 10, 64)
	if err != nil || ms <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(ms).UTC(), true
}
