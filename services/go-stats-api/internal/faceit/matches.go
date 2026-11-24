package faceit

import "context"

// GetMatchStats devolve as estatísticas de uma partida terminada.
func (c *Client) GetMatchStats(ctx context.Context, matchID string) (MatchStats, error) {
	id, err := segment("match_id", matchID)
	if err != nil {
		return MatchStats{}, err
	}
	var s MatchStats
	err = c.get(ctx, "/matches/"+id+"/stats", nil, &s)
	return s, err
}
