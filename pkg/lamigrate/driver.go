package lamigrate

import (
	"context"
	"database/sql"
)

// Driver определяет операции для конкретной БД.
// Назначение: абстрагировать различия между СУБД.
// Driver defines database-specific operations.
// Purpose: abstract differences between database backends.
type Driver interface {
	Name() string
	Open(dsn string) (*sql.DB, error)
	EnsureSchema(ctx context.Context, db *sql.DB) error
	AppliedMigrations(ctx context.Context, db *sql.DB) ([]AppliedMigration, error)
	MaxStage(ctx context.Context, db *sql.DB) (int, error)
	StagesDesc(ctx context.Context, db *sql.DB) ([]int, error)
	MigrationsByStage(ctx context.Context, db *sql.DB, stage int) ([]string, error)
	WithTransaction(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error
	InsertMigration(ctx context.Context, tx *sql.Tx, migrationName string, stage int) error
	DeleteMigration(ctx context.Context, tx *sql.Tx, migrationName string) error
}

// AppliedMigration — запись о применённой миграции со stage.
// Назначение: отдавать список применённых миграций для status/планирования.
// AppliedMigration is a stored migration record with stage.
// Purpose: return applied migrations for status/planning.
type AppliedMigration struct {
	Migration string
	Stage     int
}
