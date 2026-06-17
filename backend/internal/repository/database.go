package repository

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Database struct {
	Pool *pgxpool.Pool
}

var db *Database

func InitDB(dsn string, maxOpenConns, maxIdleConns int) error {
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("failed to parse database config: %w", err)
	}

	if maxOpenConns > 0 {
		poolConfig.MaxConns = int32(maxOpenConns)
	}
	if maxIdleConns > 0 {
		poolConfig.MinConns = int32(maxIdleConns)
	}
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	db = &Database{Pool: pool}

	if err := db.checkTimescaleDB(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("timescaledb check failed: %w", err)
	}

	log.Println("Database initialized successfully")
	return nil
}

func CloseDB() {
	if db != nil && db.Pool != nil {
		db.Pool.Close()
		log.Println("Database connection closed")
	}
}

func (d *Database) checkTimescaleDB(ctx context.Context) error {
	var version string
	err := d.Pool.QueryRow(ctx, "SELECT extversion FROM pg_extension WHERE extname = 'timescaledb'").Scan(&version)
	if err != nil {
		return fmt.Errorf("timescaledb extension not found: %w", err)
	}
	return nil
}

func GetDB() *Database {
	return db
}

func (d *Database) HealthCheck(ctx context.Context) error {
	if d == nil || d.Pool == nil {
		return fmt.Errorf("database connection is not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := d.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}

	var result string
	err := d.Pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("database query check failed: %w", err)
	}

	stats := d.Pool.Stat()
	if stats.TotalConns() == 0 {
		return fmt.Errorf("no active connections in pool")
	}

	return nil
}

func (d *Database) GetPoolStats() map[string]interface{} {
	if d == nil || d.Pool == nil {
		return nil
	}

	stats := d.Pool.Stat()
	return map[string]interface{}{
		"total_conns":      stats.TotalConns(),
		"idle_conns":       stats.IdleConns(),
		"acquired_conns":   stats.AcquiredConns(),
		"max_conns":        stats.MaxConns(),
		"min_conns":        stats.MinConns(),
		"new_conns_count":  stats.NewConnsCount(),
		"lifetime_destroy": stats.LifetimeDestroyCount(),
		"idle_destroy":     stats.IdleDestroyCount(),
	}
}

func (d *Database) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}

func (d *Database) BeginTx(ctx context.Context) (pgx.Tx, error) {
	if d == nil || d.Pool == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}
	return d.Pool.Begin(ctx)
}
