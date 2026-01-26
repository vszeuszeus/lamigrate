package lamigrate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var migrationPattern = regexp.MustCompile(`^(\d{14})_(.+)\.(up|down)\.sql$`)

// ScanMigrations читает директорию и парсит файлы в метаданные миграций.
// Вход: путь к директории с миграциями.
// Выход: упорядоченный список Migration или error при IO/валидации.
// Назначение: получить детерминированный список для apply/rollback.
func ScanMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var migrations []Migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		match := migrationPattern.FindStringSubmatch(name)
		if match == nil {
			continue
		}

		version := match[1]
		migrationName := strings.TrimSpace(match[2])
		direction := Direction(match[3])

		if migrationName == "" {
			return nil, fmt.Errorf("invalid migration name in file: %s", name)
		}

		migrations = append(migrations, Migration{
			Version:   version,
			Name:      migrationName,
			Direction: direction,
			Filename:  name,
			Path:      filepath.Join(dir, name),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		if migrations[i].Version != migrations[j].Version {
			return migrations[i].Version < migrations[j].Version
		}
		if migrations[i].Name != migrations[j].Name {
			return migrations[i].Name < migrations[j].Name
		}
		return migrations[i].Direction < migrations[j].Direction
	})

	return migrations, nil
}
