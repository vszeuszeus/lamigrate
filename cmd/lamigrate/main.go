package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"lamigrate/pkg/lamigrate"
	"lamigrate/pkg/lamigrate/drivers/postgres"
)

// main парсит CLI-флаги и запускает нужную команду миграций.
// Вход: флаги командной строки.
// Выход: код завершения процесса и сообщения stdout/stderr.
// Назначение: простой CLI для операций миграции.
func main() {
	var (
		migrationsDir = flag.String("dir", "./migrations", "directory with migration files")
		driverName    = flag.String("driver", "postgres", "database driver name")
		dsn           = flag.String("dsn", "", "database connection string/DSN")
		command       = flag.String("command", "up", "command to run: up, down, status")
		stages        = flag.Int("stages", 1, "number of stages to rollback for down")
		timeout       = flag.Duration("timeout", 5*time.Minute, "overall migration timeout")
	)
	flag.Parse()

	// Берём значения из env по умолчанию, иначе из флагов.
	envDriver := os.Getenv("LAMIGRATE_DRIVER")
	envDSN := os.Getenv("LAMIGRATE_DSN")
	envDir := os.Getenv("LAMIGRATE_MIGRATIONS_DIR")

	if envDriver != "" {
		*driverName = envDriver
	}
	if envDSN != "" {
		*dsn = envDSN
	}
	if envDir != "" {
		*migrationsDir = envDir
	}

	if *driverName == "" {
		*driverName = "postgres"
	}

	if *dsn == "" {
		log.Fatal("dsn is required")
	}

	drivers := map[string]lamigrate.Driver{
		"postgres": postgres.New(),
	}

	driver, ok := drivers[*driverName]
	if !ok {
		log.Fatalf("unsupported driver: %s", *driverName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	cfg := lamigrate.Config{
		MigrationsDir: *migrationsDir,
		DriverName:    driver.Name(),
		DSN:           *dsn,
	}

	switch *command {
	case "up":
		if err := lamigrate.ApplyUp(ctx, cfg, driver); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	case "down":
		if err := lamigrate.ApplyDown(ctx, cfg, driver, *stages); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	case "status":
		applied, err := lamigrate.ListApplied(ctx, cfg, driver)
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
	default:
		log.Fatalf("unknown command: %s", *command)
	}
}
