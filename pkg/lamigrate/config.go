package lamigrate

// Config хранит настройки для запуска миграций.
// Назначение: передать DSN и директорию в функции запуска.
// Config holds settings for running migrations.
// Purpose: pass DSN and directory into runner functions.
type Config struct {
	MigrationsDir string
	DriverName    string
	DSN           string
}
