package database

import (
	"fmt"
	"github.com/jmoiron/sqlx"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(config Config) error {
	// Connect to the database for migrations
	db, err := NewPostgresDB(config)
	if err != nil {
		return fmt.Errorf("failed to connect to database for migrations: %w", err)
	}
	defer func(db *sqlx.DB) {
		err := db.Close()
		if err != nil {
			fmt.Printf("failed to close database connection: %v", err)
		}
	}(db)

	// Get the underlying *sql.DB from sqlx.DB
	sqlDB := db.DB

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://migrations",
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migration instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	slog.Info("Database migrations completed successfully")
	return nil
}
