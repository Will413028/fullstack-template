// Package db opens the PostgreSQL connection pool.
package db

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// lockTimeout bounds how long a statement waits for a lock, so DDL queued behind a long transaction cannot stall
// every query behind it. idleInTxTimeout ends transactions a caller forgot to finish; it is longer than the statement
// timeout because a transaction legitimately spans several statements.
const (
	lockTimeout     = 3 * time.Second
	idleInTxTimeout = 30 * time.Second
)

// Config configures the pool.
type Config struct {
	// URL has no password; Password is set on the connection config so it never appears in a URL or error.
	URL              string
	Password         string
	MaxConns         int32
	StatementTimeout time.Duration
}

// Open creates the pool. pgxpool connects lazily, so a database that is briefly unavailable does not fail
// startup; readiness (Pool.Ping) reports it instead.
//
// Every connection sets statement_timeout, lock_timeout, idle_in_transaction_session_timeout and TimeZone=UTC, so
// a runaway query, a lock queue or an abandoned transaction cannot hold a connection.
func Open(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pc, err := poolConfig(cfg)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(ctx, pc)
}

func poolConfig(cfg Config) (*pgxpool.Config, error) {
	pc, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	pc.ConnConfig.Password = cfg.Password
	pc.MaxConns = cfg.MaxConns
	params := pc.ConnConfig.RuntimeParams
	params["statement_timeout"] = millis(cfg.StatementTimeout)
	params["lock_timeout"] = millis(lockTimeout)
	params["idle_in_transaction_session_timeout"] = millis(idleInTxTimeout)
	params["TimeZone"] = "UTC"
	return pc, nil
}

func millis(d time.Duration) string { return strconv.FormatInt(d.Milliseconds(), 10) }
