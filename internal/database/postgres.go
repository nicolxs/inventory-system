package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // PostgreSQL driver for migrate
	_ "github.com/golang-migrate/migrate/v4/source/file"       // File source for migrate
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectPostgres establishes a connection pool to PostgreSQL
func ConnectPostgres(dbSourceURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dbSourceURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse database_url: %w", err)
	}

	// You can configure pool settings here if needed
	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	config.HealthCheckPeriod = time.Minute
	config.ConnConfig.ConnectTimeout = 5 * time.Second

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Ping the database to ensure connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	log.Println("Successfully connected to PostgreSQL database!")
	return pool, nil
}

// RunMigrations applies database migrations
// migrationURL: e.g., "file://./migrations"
// dbSourceURL: e.g., "postgresql://user:pass@host:port/dbname?sslmode=disable"
func RunMigrations(migrationURL string, dbSourceURL string) {
	if migrationURL == "" {
		log.Println("MIGRATION_URL is not set, skipping migrations.")
		return
	}
	// The dbSourceURL for migrate needs to be slightly different if it has `pgx` specific params.
	// Often, the same DSN used for pgxpool works, but sometimes it needs to be the simpler lib/pq format.
	// For this example, we'll assume the pgx DSN works.
	// If you encounter "unknown driver pgx" or similar, ensure you have the `database/postgres` import.
	m, err := migrate.New(migrationURL, dbSourceURL)
	if err != nil {
		log.Fatalf("Cannot create new migrate instance: %v", err)
	}

	if err = m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Error running migrations up: %v", err)
	}

	if err == migrate.ErrNoChange {
		log.Println("No new migrations to apply.")
	} else {
		log.Println("Database migrations applied successfully!")
	}

	// Check for migration errors (source and database versions)
	// srcErr, dbErr := m.Close()
	// if srcErr != nil {
	//  log.Printf("Migration source error: %v", srcErr)
	// }
	// if dbErr != nil {
	//  log.Printf("Migration database error: %v", dbErr)
	// }
}
