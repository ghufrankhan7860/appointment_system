// Package datastore owns SQL client construction, pool policy, transaction primitives,
// migration execution hooks, readiness checks, and shutdown.
//
// Configure explicit connection and query timeouts, pool limits, context propagation, and
// slow-query telemetry. Return narrow infrastructure types to composition roots; do not
// place appointment queries or business rules here.
package datastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Config contains the PostgreSQL connection and pool policy selected at startup.
type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnectTimeout  time.Duration
}

// Store owns the GORM and underlying SQL database handles.
type Store struct {
	DB    *gorm.DB
	sqlDB *sql.DB
}

// Open connects to PostgreSQL, configures the pool, and verifies readiness.
func Open(ctx context.Context, config Config) (*Store, error) {
	dsn := (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(config.User, config.Password),
		Host:   config.Host + ":" + strconv.Itoa(config.Port),
		Path:   config.Database,
		RawQuery: url.Values{
			"sslmode":  []string{"disable"},
			"TimeZone": []string{"UTC"},
		}.Encode(),
	}).String()

	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}

	sqlDB, err := database.DB()
	if err != nil {
		return nil, fmt.Errorf("get SQL database: %w", err)
	}
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)

	store := &Store{DB: database, sqlDB: sqlDB}
	pingContext, cancel := context.WithTimeout(ctx, config.ConnectTimeout)
	defer cancel()
	if err := store.Ping(pingContext); err != nil {
		return nil, errors.Join(fmt.Errorf("ping PostgreSQL: %w", err), sqlDB.Close())
	}

	return store, nil
}

// Ping verifies that PostgreSQL can serve requests.
func (store *Store) Ping(ctx context.Context) error {
	return store.sqlDB.PingContext(ctx)
}

// Close releases all database connections.
func (store *Store) Close() error {
	return store.sqlDB.Close()
}
