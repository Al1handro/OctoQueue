package migrator

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrator структура для применения миграций.
type Migrator struct {
	srcDriver source.Driver
}

// MustGetNewMigrator создает новый экземпляр Migrator с встроенными SQL-файлами миграций.
// В случае ошибки вызывает panic.
func MustGetNewMigrator(sqlFiles embed.FS, dirName string) *Migrator {
	d, err := iofs.New(sqlFiles, dirName)
	if err != nil {
		panic(err)
	}
	return &Migrator{
		srcDriver: d,
	}
}

func (m *Migrator) ApplyMigrations(db *sql.DB) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf(
			"unable to create db instance: %w",
			err,
		)
	}

	migrator, err := migrate.NewWithInstance(
		"migration_embeded_sql_files",
		m.srcDriver,
		"psql_db",
		driver,
	)
	if err != nil {
		return fmt.Errorf(
			"unable to create migration instance: %w",
			err,
		)
	}

	defer func() {
		_, _ = migrator.Close()
	}()

	if err := migrator.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}

		var dirty migrate.ErrDirty
		if errors.As(err, &dirty) {
			return fmt.Errorf(
				"database is dirty at version %d",
				dirty.Version,
			)
		}

		return fmt.Errorf(
			"unable to apply migrations: %w",
			err,
		)
	}

	return nil
}