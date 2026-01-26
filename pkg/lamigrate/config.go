package lamigrate

// Config хранит настройки для запуска миграций.
// Назначение: передать DSN и директорию в функции запуска.
type Config struct {
	MigrationsDir string
	DriverName    string
	DSN           string
}
