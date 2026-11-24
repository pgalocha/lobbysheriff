-- Schema inicial do go-stats-api.
-- Lobbies e associações jogador↔lobby pertencem ao php-players-api; aqui só
-- existe o lobby_id (sem FK), porque cada serviço tem a sua base de dados.

CREATE TABLE players (
    id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    faceit_player_id CHAR(36)        NOT NULL,
    nickname         VARCHAR(64)     NOT NULL,
    avatar_url       VARCHAR(512)    NULL,
    country          CHAR(2)         NULL,
    last_synced_at   DATETIME(3)     NULL,
    created_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_players_faceit_player_id (faceit_player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Estado do jogador em cada sincronização com a FACEIT. Só é inserido um novo
-- snapshot quando content_hash muda ("se for igual, não atualiza").
CREATE TABLE player_snapshots (
    id             BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT,
    player_id      BIGINT UNSIGNED  NOT NULL,
    elo            INT UNSIGNED     NOT NULL,
    skill_level    TINYINT UNSIGNED NOT NULL,
    matches_played INT UNSIGNED     NOT NULL,
    win_rate       DECIMAL(5,2)     NOT NULL COMMENT 'percentagem 0-100',
    kd_ratio       DECIMAL(5,2)     NOT NULL,
    adr            DECIMAL(6,2)     NULL     COMMENT 'nem sempre disponível nas stats lifetime',
    headshot_pct   DECIMAL(5,2)     NOT NULL COMMENT 'percentagem 0-100',
    content_hash   BINARY(32)       NOT NULL COMMENT 'SHA-256 dos valores acima',
    captured_at    DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_player_snapshots_player_captured (player_id, captured_at),
    CONSTRAINT fk_player_snapshots_player
        FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE matches (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    faceit_match_id VARCHAR(64)     NOT NULL,
    map             VARCHAR(32)     NOT NULL,
    game_mode       VARCHAR(32)     NULL,
    started_at      DATETIME(3)     NULL,
    finished_at     DATETIME(3)     NOT NULL,
    created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_matches_faceit_match_id (faceit_match_id),
    KEY idx_matches_finished_at (finished_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Estatísticas de cada jogador seguido numa partida.
CREATE TABLE player_match_stats (
    match_id     BIGINT UNSIGNED   NOT NULL,
    player_id    BIGINT UNSIGNED   NOT NULL,
    won          BOOLEAN           NOT NULL,
    kills        SMALLINT UNSIGNED NOT NULL,
    deaths       SMALLINT UNSIGNED NOT NULL,
    assists      SMALLINT UNSIGNED NOT NULL,
    headshots    SMALLINT UNSIGNED NOT NULL,
    mvps         TINYINT UNSIGNED  NOT NULL,
    kd_ratio     DECIMAL(5,2)      NOT NULL,
    adr          DECIMAL(6,2)      NULL,
    headshot_pct DECIMAL(5,2)      NOT NULL COMMENT 'percentagem 0-100',
    PRIMARY KEY (match_id, player_id),
    KEY idx_player_match_stats_player (player_id),
    CONSTRAINT fk_player_match_stats_match
        FOREIGN KEY (match_id) REFERENCES matches (id) ON DELETE CASCADE,
    CONSTRAINT fk_player_match_stats_player
        FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- LobbySheriff da Semana: um vencedor por lobby e por semana (segunda-feira).
CREATE TABLE weekly_sheriffs (
    id             BIGINT UNSIGNED   NOT NULL AUTO_INCREMENT,
    lobby_id       BIGINT UNSIGNED   NOT NULL COMMENT 'id do lobby no php-players-api (sem FK)',
    week_start     DATE              NOT NULL COMMENT 'segunda-feira da semana',
    player_id      BIGINT UNSIGNED   NOT NULL,
    score          DECIMAL(8,2)      NOT NULL,
    matches_played SMALLINT UNSIGNED NOT NULL,
    calculated_at  DATETIME(3)       NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_weekly_sheriffs_lobby_week (lobby_id, week_start),
    KEY idx_weekly_sheriffs_player (player_id),
    CONSTRAINT fk_weekly_sheriffs_player
        FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
