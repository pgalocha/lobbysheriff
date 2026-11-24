// Package config carrega a configuração do serviço a partir de variáveis de ambiente.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
)

// Config é a configuração do go-stats-api.
type Config struct {
	Env   string
	Port  int
	MySQL MySQL
	Redis Redis
}

// MySQL é a ligação ao schema do go-stats-api.
type MySQL struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

// Addr devolve host:porta do MySQL.
func (m MySQL) Addr() string {
	return net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
}

// Redis é a ligação ao Redis.
type Redis struct {
	Host     string
	Port     int
	Password string
}

// Addr devolve host:porta do Redis.
func (r Redis) Addr() string {
	return net.JoinHostPort(r.Host, strconv.Itoa(r.Port))
}

// Load lê a configuração do ambiente, aplicando valores por omissão.
// Devolve todos os erros de uma vez, para não ser preciso corrigir um a um.
func Load() (Config, error) {
	var errs []error
	l := loader{errs: &errs}

	cfg := Config{
		Env:  l.string("APP_ENV", "local"),
		Port: l.port("GO_STATS_API_PORT", 8080),
		MySQL: MySQL{
			Host:     l.string("MYSQL_HOST", "mysql"),
			Port:     l.port("MYSQL_PORT", 3306),
			Database: l.required("STATS_DB_NAME"),
			User:     l.required("STATS_DB_USER"),
			Password: l.required("STATS_DB_PASSWORD"),
		},
		Redis: Redis{
			Host:     l.string("REDIS_HOST", "redis"),
			Port:     l.port("REDIS_PORT", 6379),
			Password: l.string("REDIS_PASSWORD", ""),
		},
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Addr devolve o endereço onde o servidor HTTP escuta.
func (c Config) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}

type loader struct {
	errs *[]error
}

func (l loader) string(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func (l loader) required(key string) string {
	v := os.Getenv(key)
	if v == "" {
		*l.errs = append(*l.errs, fmt.Errorf("%s é obrigatória", key))
	}
	return v
}

func (l loader) port(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*l.errs = append(*l.errs, fmt.Errorf("%s inválida %q: %w", key, v, err))
		return 0
	}
	if n < 1 || n > 65535 {
		*l.errs = append(*l.errs, fmt.Errorf("%s fora do intervalo 1-65535: %d", key, n))
		return 0
	}
	return n
}
