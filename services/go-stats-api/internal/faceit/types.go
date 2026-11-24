package faceit

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
	Status          string `json:"status"`
	StartedAt       int64  `json:"started_at"`
	FinishedAt      int64  `json:"finished_at"`
	FaceitURL       string `json:"faceit_url"`
}

// MatchStats são as estatísticas de uma partida (GET /matches/{match_id}/stats).
// Uma partida pode ter vários mapas (rounds), por ex. num BO3.
type MatchStats struct {
	Rounds []MatchRound `json:"rounds"`
}

// MatchRound é um mapa jogado. As chaves de RoundStats/TeamStats/PlayerStats
// são as da FACEIT ("Map", "Score", "Kills", "K/D Ratio", ...), valores em string.
type MatchRound struct {
	MatchID    string            `json:"match_id"`
	RoundStats map[string]string `json:"round_stats"`
	Teams      []MatchTeam       `json:"teams"`
}

// MatchTeam é uma equipa num mapa.
type MatchTeam struct {
	TeamID    string            `json:"team_id"`
	TeamStats map[string]string `json:"team_stats"`
	Players   []MatchPlayer     `json:"players"`
}

// MatchPlayer são as estatísticas de um jogador num mapa.
type MatchPlayer struct {
	PlayerID    string            `json:"player_id"`
	Nickname    string            `json:"nickname"`
	PlayerStats map[string]string `json:"player_stats"`
}
