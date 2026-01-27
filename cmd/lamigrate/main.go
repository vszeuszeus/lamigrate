package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lamigrate/pkg/lamigrate"
	"lamigrate/pkg/lamigrate/drivers/postgres"
)

// version содержит текущую версию CLI.
// Назначение: показывать версию в команде version.
// version holds the current CLI version.
// Purpose: print version in the version command.
var version = "0.1.3"

// main парсит CLI-флаги и запускает нужную команду миграций.
// Вход: флаги командной строки.
// Выход: код завершения процесса и сообщения stdout/stderr.
// Назначение: простой CLI для операций миграции.
// main parses CLI flags and runs a selected migration command.
// Input: command-line flags.
// Output: process exit code and stdout/stderr messages.
// Purpose: provide a simple CLI for migration operations.
func main() {
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		handleSubcommand(os.Args[1:])
		return
	}

	handleLegacyFlags()
}

// handleSubcommand обрабатывает команды вида "lamigrate <command>".
// Вход: args (аргументы без имени бинарника).
// Выход: печатает результат или завершает процесс.
// Назначение: поддержать удобный синтаксис без флага -command.
// handleSubcommand handles "lamigrate <command>" style.
// Input: args (arguments without binary name).
// Output: prints result or exits.
// Purpose: support command-first syntax.
func handleSubcommand(args []string) {
	switch args[0] {
	case "help", "-h", "--help":
		printHelp()
		return
	case "version", "-v", "--version":
		fmt.Println(version)
		return
	}

	fs := flag.NewFlagSet("lamigrate", flag.ExitOnError)
	cfg := configFlags(fs)

	switch args[0] {
	case "up":
		_ = fs.Parse(args[1:])
		runUp(cfg)
	case "down":
		stages := fs.Int("stages", 1, "сколько стадий откатить (только для down)")
		_ = fs.Parse(args[1:])
		runDown(cfg, *stages)
	case "status":
		_ = fs.Parse(args[1:])
		runStatus(cfg)
	case "create":
		nameFlag := fs.String("name", "", "имя миграции (если не указано, берётся первый аргумент)")
		_ = fs.Parse(args[1:])
		name := *nameFlag
		if name == "" && len(fs.Args()) > 0 {
			name = fs.Args()[0]
		}
		runCreate(cfg, name)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printHelp()
		os.Exit(2)
	}
}

// handleLegacyFlags обрабатывает старый формат с флагом -command.
// Вход: флаги из os.Args.
// Выход: печатает результат или завершает процесс.
// Назначение: сохранить обратную совместимость.
// handleLegacyFlags handles legacy -command style flags.
// Input: flags from os.Args.
// Output: prints result or exits.
// Purpose: keep backward compatibility.
func handleLegacyFlags() {
	var (
		command = flag.String("command", "up", "command to run: up, down, status, create")
		stages  = flag.Int("stages", 1, "number of stages to rollback for down")
		name    = flag.String("name", "", "migration name for create")
	)
	cfg := configFlags(flag.CommandLine)
	flag.Parse()

	switch *command {
	case "up":
		runUp(cfg)
	case "down":
		runDown(cfg, *stages)
	case "status":
		runStatus(cfg)
	case "create":
		runCreate(cfg, *name)
	default:
		log.Fatalf("unknown command: %s", *command)
	}
}

// configFlags регистрирует флаги конфигурации и возвращает структуру.
// Вход: FlagSet для регистрации флагов.
// Выход: указатель на config.
// Назначение: централизовать объявление флагов.
// configFlags registers configuration flags and returns the config struct.
// Input: FlagSet to register flags on.
// Output: pointer to config.
// Purpose: centralize flag definitions.
func configFlags(fs *flag.FlagSet) *config {
	cfg := &config{}
	fs.StringVar(&cfg.migrationsDir, "dir", "./migrations", "directory with migration files")
	fs.StringVar(&cfg.driverName, "driver", "postgres", "database driver name")
	fs.StringVar(&cfg.dsn, "dsn", "", "database connection string/DSN")
	fs.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "overall migration timeout")
	return cfg
}

// config хранит значения флагов/окружения до финальной сборки.
// Назначение: промежуточная структура для CLI.
// config holds flag/env values before final resolution.
// Purpose: intermediate structure for CLI.
type config struct {
	migrationsDir string
	driverName    string
	dsn           string
	timeout       time.Duration
}

// runUp запускает применение up-миграций.
// Вход: cfg с флагами/окружением.
// Выход: завершает процесс при ошибке.
// Назначение: выполнить команду up.
// runUp runs applying up migrations.
// Input: cfg with flags/env.
// Output: exits process on error.
// Purpose: execute the up command.
func runUp(cfg *config) {
	driver, config := buildConfig(cfg, true)
	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()

	start := time.Now()
	applied, err := lamigrate.ApplyUp(ctx, config.cfg, driver)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if len(applied) == 0 {
		fmt.Println("no changes")
		fmt.Printf("status: applied 0 migrations in %s\n", time.Since(start).Truncate(time.Millisecond))
		return
	}
	for _, name := range applied {
		fmt.Println(name)
	}
	fmt.Printf("status: applied %d migrations in %s\n", len(applied), time.Since(start).Truncate(time.Millisecond))
}

// runDown запускает откат стадий.
// Вход: cfg с флагами/окружением, stages количество стадий.
// Выход: завершает процесс при ошибке.
// Назначение: выполнить команду down.
// runDown runs stage rollback.
// Input: cfg with flags/env, stages count.
// Output: exits process on error.
// Purpose: execute the down command.
func runDown(cfg *config, stages int) {
	driver, config := buildConfig(cfg, true)
	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()

	start := time.Now()
	applied, err := lamigrate.ApplyDown(ctx, config.cfg, driver, stages)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if len(applied) == 0 {
		fmt.Println("no changes")
		fmt.Printf("status: rolled back 0 migrations in %s\n", time.Since(start).Truncate(time.Millisecond))
		return
	}
	for _, name := range applied {
		fmt.Println(name)
	}
	fmt.Printf("status: rolled back %d migrations in %s\n", len(applied), time.Since(start).Truncate(time.Millisecond))
}

// runStatus выводит список применённых миграций.
// Вход: cfg с флагами/окружением.
// Выход: печать результата или завершение при ошибке.
// Назначение: выполнить команду status.
// runStatus prints applied migrations.
// Input: cfg with flags/env.
// Output: prints results or exits on error.
// Purpose: execute the status command.
func runStatus(cfg *config) {
	driver, config := buildConfig(cfg, false)
	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()

	applied, err := lamigrate.ListApplied(ctx, config.cfg, driver)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if len(applied) == 0 {
		fmt.Println("no migrations applied")
		return
	}
	for _, item := range applied {
		fmt.Printf("stage=%d migration=%s\n", item.Stage, item.Migration)
	}
}

// runCreate создаёт пару файлов миграции (up/down) с текущей меткой времени.
// Вход: cfg с флагами/окружением, name имя миграции.
// Выход: печать результата или завершение при ошибке.
// Назначение: выполнить команду create.
// runCreate creates up/down migration files with current timestamp.
// Input: cfg with flags/env, name of migration.
// Output: prints result or exits on error.
// Purpose: execute the create command.
func runCreate(cfg *config, name string) {
	_, config := buildConfig(cfg, true)
	if strings.TrimSpace(name) == "" {
		fmt.Fprintln(os.Stderr, "migration name is required")
		os.Exit(1)
	}
	if err := createMigrationFiles(config.cfg.MigrationsDir, name); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

// resolvedConfig содержит итоговую конфигурацию и таймаут.
// Назначение: единый контейнер для запуска.
// resolvedConfig holds the final config and timeout.
// Purpose: a single container for execution.
type resolvedConfig struct {
	cfg     lamigrate.Config
	timeout time.Duration
}

// buildConfig собирает конфигурацию из env и флагов.
// Вход: cfg из флагов, requireDir — нужна ли директория миграций.
// Выход: драйвер и итоговый config.
// Назначение: применить приоритет env и собрать DSN.
// buildConfig builds config from env and flags.
// Input: cfg from flags, requireDir whether migrations dir is required.
// Output: driver and resolved config.
// Purpose: apply env priority and build DSN.
func buildConfig(cfg *config, requireDir bool) (lamigrate.Driver, resolvedConfig) {
	driverName := pickEnv("LAMIGRATE_DRIVER", cfg.driverName)
	if driverName == "" {
		driverName = "postgres"
	}

	migrationsDir := pickEnv("LAMIGRATE_MIGRATIONS_DIR", cfg.migrationsDir)
	dsn := pickEnv("LAMIGRATE_DSN", cfg.dsn)
	if dsn == "" {
		dsn = buildPostgresDSNFromEnv()
	}

	if dsn == "" {
		log.Fatal("dsn is required")
	}
	if requireDir && migrationsDir == "" {
		log.Fatal("migrations dir is required")
	}

	drivers := map[string]lamigrate.Driver{
		"postgres": postgres.New(),
	}

	driver, ok := drivers[driverName]
	if !ok {
		log.Fatalf("unsupported driver: %s", driverName)
	}

	return driver, resolvedConfig{
		cfg: lamigrate.Config{
			MigrationsDir: migrationsDir,
			DriverName:    driver.Name(),
			DSN:           dsn,
		},
		timeout: cfg.timeout,
	}
}

// pickEnv возвращает env значение или fallback.
// Вход: имя переменной и fallback.
// Выход: строка.
// Назначение: единый приоритет env над флагами.
// pickEnv returns env value or fallback.
// Input: variable name and fallback.
// Output: string.
// Purpose: unify env-over-flags priority.
func pickEnv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// buildPostgresDSNFromEnv строит DSN из POSTGRES_*.
// Вход: env переменные POSTGRES_*.
// Выход: строка DSN или пустая строка.
// Назначение: позволить подключаться без прямого DSN.
// buildPostgresDSNFromEnv builds a DSN from POSTGRES_*.
// Input: POSTGRES_* env variables.
// Output: DSN string or empty.
// Purpose: allow connecting without explicit DSN.
func buildPostgresDSNFromEnv() string {
	host := os.Getenv("POSTGRES_HOST")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	db := os.Getenv("POSTGRES_DB")
	port := os.Getenv("POSTGRES_PORT")

	if host == "" && user == "" && db == "" {
		return ""
	}
	if host == "" || user == "" || db == "" {
		return ""
	}
	if port == "" {
		port = "5432"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return ""
	}

	if password != "" {
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, db)
	}
	return fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=disable", user, host, port, db)
}

// createMigrationFiles создаёт файлы up/down миграции по стандартному имени.
// Вход: migrationsDir и name миграции.
// Выход: error при ошибке создания.
// Назначение: генерация заготовок миграций на диске.
// createMigrationFiles creates up/down migration files with standard naming.
// Input: migrationsDir and migration name.
// Output: error on creation failure.
// Purpose: generate migration templates on disk.
func createMigrationFiles(migrationsDir, name string) error {
	timestamp := time.Now().Format("20060102150405")
	safeName := strings.TrimSpace(name)
	safeName = strings.ReplaceAll(safeName, " ", "_")

	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		return fmt.Errorf("create migrations dir: %w", err)
	}

	upFile := fmt.Sprintf("%s_%s.up.sql", timestamp, safeName)
	downFile := fmt.Sprintf("%s_%s.down.sql", timestamp, safeName)

	upPath := filepath.Join(migrationsDir, upFile)
	downPath := filepath.Join(migrationsDir, downFile)

	if err := createEmptyFile(upPath); err != nil {
		return fmt.Errorf("create up migration: %w", err)
	}
	if err := createEmptyFile(downPath); err != nil {
		return fmt.Errorf("create down migration: %w", err)
	}

	fmt.Println(upFile)
	fmt.Println(downFile)
	return nil
}

// createEmptyFile создаёт пустой файл, если он не существует.
// Вход: путь к файлу.
// Выход: error при ошибке создания.
// Назначение: безопасно создавать файлы миграций.
// createEmptyFile creates an empty file if it does not exist.
// Input: file path.
// Output: error on creation failure.
// Purpose: safely create migration files.
func createEmptyFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

// printHelp печатает справку по CLI.
// Вход: нет.
// Выход: печатает help в stdout.
// Назначение: показать документацию команд и флагов.
// printHelp prints CLI help.
// Input: none.
// Output: help text to stdout.
// Purpose: show command and flag documentation.
func printHelp() {
	fmt.Print(`lamigrate — CLI для миграций

Использование:
  lamigrate <command> [flags]

Команды:
  up        применить все новые up-миграции в одной транзакции
  down      откатить последние стадии (по умолчанию 1)
  status    показать применённые миграции
  create    создать пару файлов миграций (up/down)
  version   показать версию
  help      показать справку

Флаги:
  -dir      путь к директории миграций (по умолчанию ./migrations)
  -driver   имя драйвера (по умолчанию postgres)
  -dsn      строка подключения к БД (или POSTGRES_* по умолчанию)
  -stages   сколько стадий откатить (только для down)
  -name     имя миграции (для create)
  -timeout  общий таймаут выполнения

Переменные окружения:
  LAMIGRATE_DSN
  LAMIGRATE_DRIVER
  LAMIGRATE_MIGRATIONS_DIR
  POSTGRES_HOST
  POSTGRES_PORT
  POSTGRES_USER
  POSTGRES_PASSWORD
  POSTGRES_DB
  
Если LAMIGRATE_DSN не задан, DSN собирается из POSTGRES_* (по умолчанию postgres).

Примеры:
  lamigrate up
  lamigrate down -stages 3
  lamigrate status
  lamigrate create add_users
`)
}
