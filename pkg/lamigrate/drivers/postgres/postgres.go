package postgres

import (
	"context"
	"database/sql"
	"fmt"

	// Регистрируем драйвер Postgres.
	_ "github.com/lib/pq"

	"lamigrate/pkg/lamigrate"
)

// Driver реализует драйвер миграций для Postgres.
type Driver struct{}

// New создаёт новый экземпляр драйвера Postgres.
// Вход: нет.
// Выход: указатель на Driver.
// Назначение: конструктор для регистрации в CLI.
func New() *Driver {
	return &Driver{}
}

// Name возвращает имя драйвера.
// Вход: нет.
// Выход: строка имени драйвера.
// Назначение: идентификация драйвера в CLI и конфигах.
func (d *Driver) Name() string {
	return "postgres"
}

// Open открывает подключение к Postgres.
// Вход: строка DSN.
// Выход: *sql.DB или error.
// Назначение: создать подключение для выполнения миграций.
func (d *Driver) Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// EnsureSchema создаёт таблицу lamigrate, если её нет.
// Вход: ctx для отмены, db соединение.
// Выход: error при ошибке создания.
// Назначение: подготовить хранилище стадий.
func (d *Driver) EnsureSchema(ctx context.Context, db *sql.DB) error {
	query := `
CREATE TABLE IF NOT EXISTS lamigrate (
	id BIGSERIAL PRIMARY KEY,
	migration TEXT NOT NULL UNIQUE,
	stage INT NOT NULL
);`
	_, err := db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("create lamigrate table: %w", err)
	}
	return nil
}

// AppliedMigrations возвращает применённые миграции, отсортированные по stage и id.
// Вход: ctx для отмены, db соединение.
// Выход: список AppliedMigration или error.
// Назначение: показать статус и проверить, что ещё не выполнено.
func (d *Driver) AppliedMigrations(ctx context.Context, db *sql.DB) ([]lamigrate.AppliedMigration, error) {
	rows, err := db.QueryContext(ctx, `SELECT migration, stage FROM lamigrate ORDER BY stage ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var applied []lamigrate.AppliedMigration
	for rows.Next() {
		var migration string
		var stage int
		if err := rows.Scan(&migration, &stage); err != nil {
			return nil, err
		}

		applied = append(applied, lamigrate.AppliedMigration{
			Migration: migration,
			Stage:     stage,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return applied, nil
}

// MaxStage возвращает максимальный stage.
// Вход: ctx для отмены, db соединение.
// Выход: максимальный stage или 0 если записей нет; error при ошибке.
// Назначение: вычислить следующий stage для batch apply.
func (d *Driver) MaxStage(ctx context.Context, db *sql.DB) (int, error) {
	var maxStage sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(stage) FROM lamigrate`).Scan(&maxStage); err != nil {
		return 0, err
	}
	if !maxStage.Valid {
		return 0, nil
	}
	return int(maxStage.Int64), nil
}

// StagesDesc возвращает список стадий по убыванию.
// Вход: ctx для отмены, db соединение.
// Выход: список стадий по убыванию или error.
// Назначение: определить порядок отката down-миграций.
func (d *Driver) StagesDesc(ctx context.Context, db *sql.DB) ([]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT stage FROM lamigrate ORDER BY stage DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stages []int
	for rows.Next() {
		var stage int
		if err := rows.Scan(&stage); err != nil {
			return nil, err
		}
		stages = append(stages, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stages, nil
}

// MigrationsByStage возвращает миграции для stage в обратном порядке.
// Вход: ctx для отмены, db соединение, номер stage.
// Выход: список имён миграций или error.
// Назначение: откатывать stage в порядке, обратном применению.
func (d *Driver) MigrationsByStage(ctx context.Context, db *sql.DB, stage int) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT migration FROM lamigrate WHERE stage = $1 ORDER BY id DESC`, stage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []string
	for rows.Next() {
		var migration string
		if err := rows.Scan(&migration); err != nil {
			return nil, err
		}
		migrations = append(migrations, migration)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return migrations, nil
}

// WithTransaction выполняет функцию в транзакции.
// Вход: ctx для отмены, db соединение, функция.
// Выход: error при ошибке транзакции или функции.
// Назначение: объединить несколько операций в одну атомарную.
func (d *Driver) WithTransaction(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// InsertMigration записывает факт применения миграции.
// Вход: ctx для отмены, tx транзакция, имя миграции, номер stage.
// Выход: error при ошибке вставки.
// Назначение: сохранить информацию о применённой миграции.
func (d *Driver) InsertMigration(ctx context.Context, tx *sql.Tx, migrationName string, stage int) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO lamigrate (migration, stage) VALUES ($1, $2)`,
		migrationName,
		stage,
	)
	return err
}

// DeleteMigration удаляет запись о миграции.
// Вход: ctx для отмены, tx транзакция, имя миграции.
// Выход: error при ошибке удаления.
// Назначение: убрать отметку о применении при откате.
func (d *Driver) DeleteMigration(ctx context.Context, tx *sql.Tx, migrationName string) error {
	_, err := tx.ExecContext(
		ctx,
		`DELETE FROM lamigrate WHERE migration = $1`,
		migrationName,
	)
	return err
}
