package lamigrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// ApplyUp выполняет все новые up-миграции в одной транзакции.
// Вход: ctx для отмены, cfg с DSN и директорией, реализация driver.
// Выход: error при ошибках валидации, IO, БД или выполнения миграций.
// Назначение: атомарно применить новый stage и записать его в lamigrate.
func ApplyUp(ctx context.Context, cfg Config, driver Driver) error {
	if cfg.MigrationsDir == "" {
		return fmt.Errorf("migrations dir is empty")
	}
	if cfg.DSN == "" {
		return fmt.Errorf("dsn is empty")
	}

	migrations, err := ScanMigrations(cfg.MigrationsDir)
	if err != nil {
		return err
	}

	db, err := driver.Open(cfg.DSN)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := driver.EnsureSchema(ctx, db); err != nil {
		return fmt.Errorf("ensure lamigrate schema: %w", err)
	}

	appliedList, err := driver.AppliedMigrations(ctx, db)
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}

	applied := make(map[string]struct{}, len(appliedList))
	for _, item := range appliedList {
		applied[item.Migration] = struct{}{}
	}

	var pending []Migration
	for _, migration := range migrations {
		if migration.Direction != DirectionUp {
			continue
		}

		if _, exists := applied[migration.Key()]; exists {
			continue
		}

		pending = append(pending, migration)
	}

	if len(pending) == 0 {
		return nil
	}

	stage, err := driver.MaxStage(ctx, db)
	if err != nil {
		return fmt.Errorf("read max stage: %w", err)
	}
	stage++

	for i := range pending {
		content, err := os.ReadFile(pending[i].Path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", pending[i].Filename, err)
		}

		sqlText := strings.TrimSpace(string(content))
		if sqlText == "" {
			return fmt.Errorf("migration %s is empty", pending[i].Filename)
		}

		pending[i].SQL = sqlText
	}

	if err := driver.WithTransaction(ctx, db, func(tx *sql.Tx) error {
		for _, migration := range pending {
			if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
				return fmt.Errorf("exec migration %s: %w", migration.Filename, err)
			}
			if err := driver.InsertMigration(ctx, tx, migration.Key(), stage); err != nil {
				return fmt.Errorf("record migration %s: %w", migration.Filename, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// ApplyDown откатывает одну или несколько стадий через down-миграции в одной транзакции.
// Вход: ctx для отмены, cfg с DSN и директорией, реализация driver,
// stagesToRollback — количество стадий для отката (1+).
// Выход: error при ошибках валидации, IO, БД или выполнения; nil при успехе/нет изменений.
// Назначение: безопасно откатить последние стадии.
func ApplyDown(ctx context.Context, cfg Config, driver Driver, stagesToRollback int) error {
	if stagesToRollback <= 0 {
		return fmt.Errorf("stages to rollback must be positive")
	}
	if cfg.MigrationsDir == "" {
		return fmt.Errorf("migrations dir is empty")
	}
	if cfg.DSN == "" {
		return fmt.Errorf("dsn is empty")
	}

	migrations, err := ScanMigrations(cfg.MigrationsDir)
	if err != nil {
		return err
	}

	db, err := driver.Open(cfg.DSN)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := driver.EnsureSchema(ctx, db); err != nil {
		return fmt.Errorf("ensure lamigrate schema: %w", err)
	}

	downByName := map[string]Migration{}
	for _, migration := range migrations {
		if migration.Direction != DirectionDown {
			continue
		}
		downByName[migration.Key()] = migration
	}

	stages, err := driver.StagesDesc(ctx, db)
	if err != nil {
		return fmt.Errorf("read stages: %w", err)
	}
	if len(stages) == 0 {
		return nil
	}

	if stagesToRollback > len(stages) {
		stagesToRollback = len(stages)
	}
	stages = stages[:stagesToRollback]

	var ordered []string
	for _, stage := range stages {
		names, err := driver.MigrationsByStage(ctx, db, stage)
		if err != nil {
			return fmt.Errorf("read migrations for stage %d: %w", stage, err)
		}
		ordered = append(ordered, names...)
	}

	if len(ordered) == 0 {
		return nil
	}

	if err := driver.WithTransaction(ctx, db, func(tx *sql.Tx) error {
		for _, name := range ordered {
			migration, ok := downByName[name]
			if !ok {
				return fmt.Errorf("missing down migration for %s", name)
			}

			content, err := os.ReadFile(migration.Path)
			if err != nil {
				return fmt.Errorf("read migration %s: %w", migration.Filename, err)
			}

			sqlText := strings.TrimSpace(string(content))
			if sqlText == "" {
				return fmt.Errorf("migration %s is empty", migration.Filename)
			}

			if _, err := tx.ExecContext(ctx, sqlText); err != nil {
				return fmt.Errorf("exec migration %s: %w", migration.Filename, err)
			}

			if err := driver.DeleteMigration(ctx, tx, name); err != nil {
				return fmt.Errorf("delete migration %s: %w", migration.Filename, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// ListApplied возвращает список применённых миграций со stage.
// Вход: ctx для отмены, cfg с DSN, реализация driver.
// Выход: список применённых миграций (может быть пустым) или error.
// Назначение: показать статус без выполнения миграций.
func ListApplied(ctx context.Context, cfg Config, driver Driver) ([]AppliedMigration, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("dsn is empty")
	}

	db, err := driver.Open(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := driver.EnsureSchema(ctx, db); err != nil {
		return nil, fmt.Errorf("ensure lamigrate schema: %w", err)
	}

	applied, err := driver.AppliedMigrations(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}

	return applied, nil
}
