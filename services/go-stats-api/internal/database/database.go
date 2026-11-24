// Package database trata da ligação ao MySQL e das migrations do schema.
package database

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/config"
	"github.com/pgalocha/lobbysheriff/services/go-stats-api/migrations"
)

// Open prepara o pool de ligações ao MySQL. Não liga de imediato: a primeira
// ligação só acontece no primeiro uso (ou no health check).
func Open(cfg config.MySQL) (*sql.DB, error) {
	return sql.Open("mysql", dsn(cfg, false))
}

func dsn(cfg config.MySQL, multiStatements bool) string {
	c := mysql.NewConfig()
	c.Net = "tcp"
	c.Addr = cfg.Addr()
	c.DBName = cfg.Database
	c.User = cfg.User
	c.Passwd = cfg.Password
	c.ParseTime = true
	c.MultiStatements = multiStatements
	return c.FormatDSN()
}

// Migrator aplica as migrations embebidas em migrations.FS.
type Migrator struct {
	m  *migrate.Migrate
	db *sql.DB
}

// NewMigrator liga ao MySQL com multiStatements ativo (só nesta ligação:
// cada ficheiro de migration pode ter vários statements).
func NewMigrator(cfg config.MySQL) (*Migrator, error) {
	db, err := sql.Open("mysql", dsn(cfg, true))
	if err != nil {
		return nil, fmt.Errorf("abrir MySQL: %w", err)
	}

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ler migrations: %w", err)
	}

	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ligar ao MySQL: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("preparar migrate: %w", err)
	}

	return &Migrator{m: m, db: db}, nil
}

// Up aplica todas as migrations pendentes. Não é erro se já estiver atualizado.
func (mg *Migrator) Up() error {
	if err := mg.m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// Down reverte as últimas `steps` migrations.
func (mg *Migrator) Down(steps int) error {
	if steps < 1 {
		return fmt.Errorf("número de passos inválido: %d", steps)
	}
	return mg.m.Steps(-steps)
}

// Version devolve a versão atual do schema e se ficou "dirty" (migration
// interrompida a meio, que tem de ser corrigida à mão com `migrate force`).
func (mg *Migrator) Version() (version uint, dirty bool, err error) {
	version, dirty, err = mg.m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	return version, dirty, err
}

// Close liberta a ligação ao MySQL.
func (mg *Migrator) Close() error {
	srcErr, dbErr := mg.m.Close()
	return errors.Join(srcErr, dbErr, mg.db.Close())
}
