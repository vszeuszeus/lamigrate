package lamigrate

// Migration описывает файл миграции и распарсенные метаданные.
// Назначение: хранить информацию о файле и SQL для выполнения.
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
type Direction string

const (
	// DirectionUp это миграция вверх.
	DirectionUp Direction = "up"
	// DirectionDown это миграция вниз.
	DirectionDown Direction = "down"
)

// Key возвращает уникальный идентификатор миграции без направления.
// Вход: структура миграции.
// Выход: строка формата "version_name".
// Назначение: связать up/down и хранить ключ в lamigrate.
func (m Migration) Key() string {
	return m.Version + "_" + m.Name
}
