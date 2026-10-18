// Package db opens the PostgreSQL connection pool.
package db

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config configures the pool.
type Config struct {
	URL              string
	MaxConns         int32
	StatementTimeout time.Duration
}

// Open creates the pool. pgxpool connects lazily, so a database that is briefly unavailable does not fail
// startup; readiness (Pool.Ping) reports it instead.
//
// Every connection sets statement_timeout, idle_in_transaction_session_timeout and TimeZone=UTC, so a runaway
// query or an abandoned transaction cannot hold a connection.
func Open(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pc, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	pc.MaxConns = cfg.MaxConns
	ms := strconv.FormatInt(cfg.StatementTimeout.Milliseconds(), 10)
	params := pc.ConnConfig.RuntimeParams
	params["statement_timeout"] = ms
	params["idle_in_transaction_session_timeout"] = ms
	params["TimeZone"] = "UTC"
	return pgxpool.NewWithConfig(ctx, pc)
}
