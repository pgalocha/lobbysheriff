// Comando go-stats-api: arranca o servidor HTTP.
//
// Uso:
//
//	go-stats-api              arranca o servidor
//	go-stats-api healthcheck  verifica /livez e sai com 0 (ok) ou 1 (falha);
//	                          usado pelo HEALTHCHECK da imagem (distroless, sem curl)
//	go-stats-api migrate up         aplica as migrations pendentes
//	go-stats-api migrate down [N]   reverte as últimas N migrations (por omissão 1)
//	go-stats-api migrate version    mostra a versão atual do schema
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"

	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/config"
	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/database"
	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/health"
	"github.com/pgalocha/lobbysheriff/services/go-stats-api/internal/server"
)

const (
	shutdownTimeout    = 10 * time.Second
	healthCheckTimeout = 2 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuração inválida", "error", err)
		os.Exit(1)
	}

	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "":
		if err := run(cfg, logger); err != nil {
			logger.Error("servidor terminou com erro", "error", err)
			os.Exit(1)
		}
	case "healthcheck":
		os.Exit(healthcheck(cfg.Port))
	case "migrate":
		if err := runMigrate(cfg.MySQL, os.Args[2:], logger); err != nil {
			logger.Error("migrate falhou", "error", err)
			os.Exit(1)
		}
	default:
		logger.Error("comando desconhecido", "command", cmd)
		os.Exit(2)
	}
}

func runMigrate(cfg config.MySQL, args []string, logger *slog.Logger) error {
	if len(args) == 0 {
		return errors.New("uso: migrate up | down [N] | version")
	}

	m, err := database.NewMigrator(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := m.Close(); err != nil {
			logger.Warn("erro ao fechar a ligação de migrate", "error", err)
		}
	}()

	switch args[0] {
	case "up":
		if err := m.Up(); err != nil {
			return err
		}
	case "down":
		steps := 1
		if len(args) > 1 {
			if steps, err = strconv.Atoi(args[1]); err != nil {
				return fmt.Errorf("número de passos inválido %q: %w", args[1], err)
			}
		}
		if err := m.Down(steps); err != nil {
			return err
		}
	case "version":
	default:
		return fmt.Errorf("subcomando desconhecido %q (uso: migrate up | down [N] | version)", args[0])
	}

	version, dirty, err := m.Version()
	if err != nil {
		return err
	}
	logger.Info("schema", "version", version, "dirty", dirty)
	return nil
}

func run(cfg config.Config, logger *slog.Logger) error {
	// sql.Open e redis.NewClient não abrem ligações: a app arranca mesmo com
	// o MySQL/Redis em baixo, e o /readyz reporta-o até ficarem disponíveis.
	db, err := database.Open(cfg.MySQL)
	if err != nil {
		return fmt.Errorf("configuração do MySQL: %w", err)
	}
	defer func() { _ = db.Close() }()

	// ContextTimeoutEnabled: os timeouts de leitura/escrita respeitam o contexto,
	// para o health check não ultrapassar o seu prazo com o Redis pendurado.
	rdb := redis.NewClient(&redis.Options{
		Addr:                  cfg.Redis.Addr(),
		Password:              cfg.Redis.Password,
		ContextTimeoutEnabled: true,
	})
	defer func() { _ = rdb.Close() }()

	checker := health.New(healthCheckTimeout, logger)
	checker.Add("mysql", db.PingContext)
	checker.Add("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() })

	app, err := server.New(checker, server.Options{CORSAllowedOrigins: cfg.CORSAllowedOrigins})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("a arrancar go-stats-api", "addr", cfg.Addr(), "env", cfg.Env)
		errCh <- app.Listen(cfg.Addr(), fiber.ListenConfig{DisableStartupMessage: true})
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("sinal recebido, a encerrar")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return app.ShutdownWithContext(shutdownCtx)
}

func healthcheck(port int) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/livez", port), http.NoBody)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}
