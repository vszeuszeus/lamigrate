# lamigrate

Сервис для запуска миграций из директории с файловым форматом:

`YYYYMMDDHHMMSS_migration-name.up.sql`  
`YYYYMMDDHHMMSS_migration-name.down.sql`

Где:
- `YYYYMMDDHHMMSS` — точная метка времени (год-месяц-день-час-минута-секунда)
- `migration-name` — произвольное имя миграции
- `up/down` — направление

## Что делает сервис

- Сканирует директорию миграций и находит файлы по шаблону.
- Хранит историю применённых миграций в таблице `lamigrate`.
- Выполняет **все новые** `up`-миграции за один запуск **в одной транзакции**.
- Каждому запуску `up` присваивает новый `stage` (stage = max(stage) + 1).
- Умеет откатывать 1 или несколько последних стадий (`down`) в одной транзакции.
- Умеет показывать, какие миграции уже применены (`status`).

## Структура таблицы `lamigrate`

```
id       BIGSERIAL PRIMARY KEY
migration TEXT NOT NULL UNIQUE
stage    INT NOT NULL
```

- `migration` хранит ключ вида `YYYYMMDDHHMMSS_name`
- `stage` — номер запуска `up`, в рамках которого были применены миграции

## Команды

### `up`
Применяет все новые `up`-миграции одной транзакцией и пишет stage.

```
go run ./cmd/lamigrate -command up -dir ./migrations -driver postgres -dsn "..."
```

### `down`
Откатывает 1 или N последних стадий (в обратном порядке) одной транзакцией.

```
go run ./cmd/lamigrate -command down -stages 2 -dir ./migrations -driver postgres -dsn "..."
```

### `status`
Показывает список применённых миграций с их stage.

```
go run ./cmd/lamigrate -command status -driver postgres -dsn "..."
```

Пример вывода:

```
stage=1 migration=20250101123000_init_schema
stage=1 migration=20250101123500_add_users
stage=2 migration=20250102120000_add_orders
```

## Параметры CLI

- `-command` — `up`, `down`, `status`
- `-dir` — путь к директории миграций (нужно для `up`/`down`)
- `-driver` — имя драйвера (сейчас `postgres`)
- `-dsn` — строка подключения к БД
- `-stages` — сколько стадий откатить (только для `down`, по умолчанию 1)
- `-timeout` — общий таймаут выполнения

## Переменные окружения

Если переменная окружения задана, она переопределяет соответствующий флаг.

- `LAMIGRATE_DSN` — строка подключения к БД
- `LAMIGRATE_DRIVER` — имя драйвера (по умолчанию `postgres`)
- `LAMIGRATE_MIGRATIONS_DIR` — путь к директории миграций

## Как запустить

### Требования

- Go 1.22+
- Доступ к базе данных (Postgres)
- Папка с миграциями по шаблону `YYYYMMDDHHMMSS_name.(up|down).sql`

### Быстрый старт (go run)

```
go run ./cmd/lamigrate \
  -command up \
  -dir ./migrations \
  -driver postgres \
  -dsn "postgres://user:pass@localhost:5432/db?sslmode=disable"
```

Проверка статуса:

```
go run ./cmd/lamigrate -command status -driver postgres -dsn "postgres://user:pass@localhost:5432/db?sslmode=disable"
```

Откат последней стадии:

```
go run ./cmd/lamigrate -command down -stages 1 -dir ./migrations -driver postgres -dsn "postgres://user:pass@localhost:5432/db?sslmode=disable"
```

### Сборка бинарника

```
go build -o ./bin/lamigrate ./cmd/lamigrate
```

Запуск бинарника:

```
./bin/lamigrate -command up -dir ./migrations -driver postgres -dsn "postgres://user:pass@localhost:5432/db?sslmode=disable"
```

### Docker

Сборка образа:

```
docker build -t lamigrate:local .
```

Запуск (монтируем миграции и передаём env):

```
docker run --rm \
  -e LAMIGRATE_DSN="postgres://user:pass@host.docker.internal:5432/db?sslmode=disable" \
  -e LAMIGRATE_DRIVER=postgres \
  -e LAMIGRATE_MIGRATIONS_DIR=/migrations \
  -v "$PWD/migrations:/migrations:ro" \
  lamigrate:local -command up
```

Статус:

```
docker run --rm \
  -e LAMIGRATE_DSN="postgres://user:pass@host.docker.internal:5432/db?sslmode=disable" \
  lamigrate:local -command status
```

### Релизный флоу (для использования в других языках)

- Собираем бинарники под OS/ARCH (linux/darwin/windows).
- Публикуем релиз в GitHub Releases.
- Используем CLI в CI/CD или напрямую в сервисах любого языка.

#### GitHub Releases (автосборка)

В репозитории есть workflow `.github/workflows/release.yml`.
Он собирает бинарники и публикует их в релиз при пуше тега `v*`.

Пример релиза:

```
git tag v0.1.0
git push origin v0.1.0
```

Пример скачивания в Dockerfile:

```
ARG LAMIGRATE_VERSION=v0.1.0
RUN curl -fsSL -o /usr/local/bin/lamigrate \
  "https://github.com/<owner>/lamigrate/releases/download/${LAMIGRATE_VERSION}/lamigrate_linux_amd64" \
  && chmod +x /usr/local/bin/lamigrate
```

Проверка sha256 (опционально):

```
curl -fsSL -o /tmp/lamigrate.sha256 \
  "https://github.com/<owner>/lamigrate/releases/download/${LAMIGRATE_VERSION}/sha256sums.txt"
```

## Поведение по стадиям

- Первый запуск `up` создаёт `stage=1`.
- Следующий `up` создаёт `stage=2` и т.д.
- `down -stages 1` откатывает только последнюю стадию.
- `down -stages N` откатывает N последних стадий в порядке убывания.

## Расширяемость

Логика работы с БД вынесена в интерфейс `Driver`.  
Чтобы добавить другую СУБД (например, MySQL), нужно реализовать драйвер и зарегистрировать его в `cmd/lamigrate/main.go`.
