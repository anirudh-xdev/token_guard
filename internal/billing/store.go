package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

const (
	defaultMaxOpenConns = 4
	defaultPingTimeout  = 2 * time.Second
)

type Config struct {
	DatabaseURL  string
	AuthToken    string
	MaxOpenConns int
	PingTimeout  time.Duration
}

type Store struct {
	db *sql.DB
}

func ConfigFromEnv() (Config, error) {
	maxOpenConns := defaultMaxOpenConns
	if raw := strings.TrimSpace(os.Getenv("TOKENGUARD_DB_MAX_OPEN_CONNS")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("TOKENGUARD_DB_MAX_OPEN_CONNS must be a positive integer")
		}
		maxOpenConns = parsed
	}

	cfg := Config{
		DatabaseURL:  strings.TrimSpace(os.Getenv("TURSO_DATABASE_URL")),
		AuthToken:    strings.TrimSpace(os.Getenv("TURSO_AUTH_TOKEN")),
		MaxOpenConns: maxOpenConns,
		PingTimeout:  defaultPingTimeout,
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var errs []error
	if strings.TrimSpace(c.DatabaseURL) == "" {
		errs = append(errs, errors.New("TURSO_DATABASE_URL is required"))
	}
	if strings.TrimSpace(c.AuthToken) == "" {
		errs = append(errs, errors.New("TURSO_AUTH_TOKEN is required"))
	}
	if c.MaxOpenConns < 0 {
		errs = append(errs, errors.New("MaxOpenConns cannot be negative"))
	}
	if c.PingTimeout < 0 {
		errs = append(errs, errors.New("PingTimeout cannot be negative"))
	}
	return errors.Join(errs...)
}

func Open(ctx context.Context, cfg Config) (*Store, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = defaultMaxOpenConns
	}
	if cfg.PingTimeout == 0 {
		cfg.PingTimeout = defaultPingTimeout
	}

	dsn, err := BuildDatabaseURL(cfg.DatabaseURL, cfg.AuthToken)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open libsql: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(max(1, cfg.MaxOpenConns/2))
	db.SetConnMaxIdleTime(time.Minute)
	db.SetConnMaxLifetime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping libsql: %w", err)
	}

	return &Store{db: db}, nil
}

func BuildDatabaseURL(databaseURL, authToken string) (string, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	authToken = strings.TrimSpace(authToken)
	if databaseURL == "" {
		return "", errors.New("database URL is required")
	}
	if authToken == "" {
		return "", errors.New("auth token is required")
	}

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse Turso database URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("Turso database URL must include scheme and host")
	}

	query := parsed.Query()
	query.Set("authToken", authToken)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (s *Store) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("billing store is nil")
	}

	for _, statement := range schemaStatements() {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("execute migration statement %q: %w", statement, err)
		}
	}
	return nil
}

func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
