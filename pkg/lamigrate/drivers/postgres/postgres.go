package postgres

import (
	"context"
	"database/sql"
	"fmt"

	// Регистрируем драйвер Postgres.
	// Register the Postgres driver.
	_ "github.com/lib/pq"

	"lamigrate/pkg/lamigrate"
)

// Driver реализует драйвер миграций для Postgres.
// Driver implements the Postgres migrations driver.
type Driver struct{}

// New создаёт новый экземпляр драйвера Postgres.
// Вход: нет.
// Выход: указатель на Driver.
// Назначение: конструктор для регистрации в CLI.
// New creates a new Postgres driver instance.
// Input: none.
// Output: pointer to Driver.
// Purpose: constructor for CLI registration.
func New() *Driver {
	return &Driver{}
}

// Name возвращает имя драйвера.
// Вход: нет.
// Выход: строка имени драйвера.
// Назначение: идентификация драйвера в CLI и конфигах.
// Name returns the driver name.
// Input: none.
// Output: driver name string.
// Purpose: identify the driver in CLI and configs.
func (d *Driver) Name() string {
	return "postgres"
}

// Open открывает подключение к Postgres.
// Вход: строка DSN.
// Выход: *sql.DB или error.
// Назначение: создать подключение для выполнения миграций.
// Open opens a Postgres connection.
// Input: DSN string.
// Output: *sql.DB or error.
// Purpose: create a connection for running migrations.
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
// EnsureSchema creates lamigrate table if missing.
// Input: ctx for cancellation, db connection.
// Output: error on creation failure.
// Purpose: prepare storage for stages.
func (d *Driver) EnsureSchema(ctx context.Context, db *sql.DB) error {
	query := `
CREATE TABLE IF NOT EXISTS lamigrate (
	id BIGSERIAL PRIMARY KEY,
	migration TEXT NOT NULL UNIQUE,
	stage INT NOT NULL,
	executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`
	_, err := db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("create lamigrate table: %w", err)
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE lamigrate ADD COLUMN IF NOT EXISTS executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`)
	if err != nil {
		return fmt.Errorf("add lamigrate executed_at column: %w", err)
	}
	_, err = db.ExecContext(ctx, `
DO $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM information_schema.columns
		WHERE table_name = 'lamigrate' AND column_name = 'executed_date'
	) THEN
		UPDATE lamigrate
		SET executed_at = executed_date
		WHERE executed_at IS NULL AND executed_date IS NOT NULL;
	END IF;
END $$;
`)
	if err != nil {
		return fmt.Errorf("backfill lamigrate executed_at: %w", err)
	}
	return nil
}

// AppliedMigrations возвращает применённые миграции, отсортированные по stage и id.
// Вход: ctx для отмены, db соединение.
// Выход: список AppliedMigration или error.
// Назначение: показать статус и проверить, что ещё не выполнено.
// AppliedMigrations returns applied migrations ordered by stage and id.
// Input: ctx for cancellation, db connection.
// Output: list of AppliedMigration or error.
// Purpose: show status and detect pending migrations.
func (d *Driver) AppliedMigrations(ctx context.Context, db *sql.DB) ([]lamigrate.AppliedMigration, error) {
	rows, err := db.QueryContext(ctx, `SELECT migration, stage, executed_at FROM lamigrate ORDER BY stage ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var applied []lamigrate.AppliedMigration
	for rows.Next() {
		var migration string
		var stage int
		var executedAt sql.NullTime
		if err := rows.Scan(&migration, &stage, &executedAt); err != nil {
			return nil, err
		}

		applied = append(applied, lamigrate.AppliedMigration{
			Migration:  migration,
			Stage:      stage,
			ExecutedAt: executedAt.Time,
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
// MaxStage returns the maximum stage.
// Input: ctx for cancellation, db connection.
// Output: max stage or 0 if none; error on failure.
// Purpose: compute next stage for batch apply.
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
// StagesDesc returns stages in descending order.
// Input: ctx for cancellation, db connection.
// Output: list of stages (desc) or error.
// Purpose: determine down rollback order.
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
// MigrationsByStage returns migrations for a stage in reverse order.
// Input: ctx for cancellation, db connection, stage number.
// Output: list of migration names or error.
// Purpose: rollback a stage in reverse apply order.
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
// WithTransaction runs a function inside a transaction.
// Input: ctx for cancellation, db connection, function.
// Output: error if transaction or function fails.
// Purpose: group multiple operations into a single atomic unit.
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
// InsertMigration records an applied migration.
// Input: ctx for cancellation, tx transaction, migration name, stage number.
// Output: error on insert failure.
// Purpose: persist applied migration info.
func (d *Driver) InsertMigration(ctx context.Context, tx *sql.Tx, migrationName string, stage int) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO lamigrate (migration, stage, executed_at) VALUES ($1, $2, NOW())`,
		migrationName,
		stage,
	)
	return err
}

// DeleteMigration удаляет запись о миграции.
// Вход: ctx для отмены, tx транзакция, имя миграции.
// Выход: error при ошибке удаления.
// Назначение: убрать отметку о применении при откате.
// DeleteMigration removes a migration record.
// Input: ctx for cancellation, tx transaction, migration name.
// Output: error on delete failure.
// Purpose: remove applied marker during rollback.
func (d *Driver) DeleteMigration(ctx context.Context, tx *sql.Tx, migrationName string) error {
	_, err := tx.ExecContext(
		ctx,
		`DELETE FROM lamigrate WHERE migration = $1`,
		migrationName,
	)
	return err
}
