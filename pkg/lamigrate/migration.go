package lamigrate

// Migration описывает файл миграции и распарсенные метаданные.
// Назначение: хранить информацию о файле и SQL для выполнения.
// Migration describes a migration file and parsed metadata.
// Purpose: hold file info and SQL for execution.
type Migration struct {
	Version   string
	Name      string
	Direction Direction
	Filename  string
	Path      string
	SQL       string
	Checksum  string
}

// Direction это направление миграции.
// Direction is a migration direction.
type Direction string

const (
	// DirectionUp это миграция вверх.
	// DirectionUp is the "up" migration direction.
	DirectionUp Direction = "up"
	// DirectionDown это миграция вниз.
	// DirectionDown is the "down" migration direction.
	DirectionDown Direction = "down"
)

// Key возвращает уникальный идентификатор миграции без направления.
// Вход: структура миграции.
// Выход: строка формата "version_name".
// Назначение: связать up/down и хранить ключ в lamigrate.
// Key returns a unique migration identifier without direction.
// Input: migration struct.
// Output: string in "version_name" format.
// Purpose: match up/down and store the key in lamigrate.
func (m Migration) Key() string {
	return m.Version + "_" + m.Name
}
