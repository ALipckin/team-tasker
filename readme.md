# Task Service

REST API сервис для управления задачами внутри команд: ролевая модель, история изменений,
комментарии, кеширование списка задач в Redis и SQL-отчёт по команде.

## Стек

- Go 1.26, `gorilla/mux`, `log/slog` (JSON-логи)
- MySQL 8.4 (`golang-migrate` — миграции при старте)
- Redis 7 (кеш списка задач)
- JWT (HS256), bcrypt
- Docker + Docker Compose
- Swagger (OpenAPI 2.0) из аннотаций swaggo/swag + Swagger UI

## Быстрый старт

```bash
cp .env.example .env      # при необходимости поменяйте JWT_SECRET и пароли
make docker-up            # dev: air + delve, код монтируется в контейнер
# или
make docker-up-prod       # prod-образ (distroless)
```

- API: `http://localhost:${APP_PORT}` (по умолчанию `8080`)
- Swagger UI: `http://localhost:8080/swagger/`
- Спека: `http://localhost:8080/swagger/doc.json` (генерируется из аннотаций хендлеров: `make swagger` → `internal/api/docs`)
- Health: `GET /health`

## Конфигурация

Вся конфигурация читается из переменных окружения (`.env` для docker compose, см. `.env.example`).

| Переменная | По умолчанию | Описание |
|---|---|---|
| `APP_PORT` | `8080` | Порт HTTP-сервера |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `DB_HOST`, `DB_PORT` | `localhost`, `3306` | MySQL (в compose — `db:3306`) |
| `DB_USER`, `DB_PASSWORD`, `DB_DATABASE` | — | Учётные данные MySQL |
| `DB_ROOT_PASSWORD` | — | Root-пароль контейнера MySQL (только compose) |
| `MIGRATIONS_PATH` | `file://internal/database/migrations` | Путь к миграциям |
| `REDIS_HOST`, `REDIS_PORT` | `localhost`, `6379` | Redis (в compose — `redis:6379`) |
| `REDIS_PASSWORD`, `REDIS_DB` | пусто, `0` | В `docker-compose.prod.yml` пароль обязателен |
| `JWT_SECRET` | — (**обязательно**) | Секрет подписи JWT |
| `JWT_TTL` | `24h` | Время жизни токена |
| `TASKS_CACHE_TTL` | `5m` | TTL кеша списка задач |
| `RATE_LIMIT_RPS`, `RATE_LIMIT_BURST` | `20`, `40` | Лимит запросов на IP (`RATE_LIMIT_RPS=0` — выключить) |
| `TRUSTED_PROXIES` | пусто | CIDR прокси через запятую, от которых принимается `X-Forwarded-For` для определения IP клиента |
| `DB_EXPOSE_PORT`, `REDIS_EXPOSE_PORT`, `DLV_PORT` | `3306`, `6379`, `40000` | Порты, пробрасываемые на хост |

## Миграции

Миграции лежат в `internal/database/migrations` и применяются автоматически при старте приложения
(`golang-migrate`, таблица `schema_migrations`). Откат вручную:

```bash
docker run --rm --network <project>_default -v "$PWD/internal/database/migrations:/m" migrate/migrate \
  -path=/m -database "mysql://$DB_USER:$DB_PASSWORD@tcp(db:3306)/$DB_DATABASE" down 1
```

Схема: `users`, `teams`, `team_members` (PK `team_id + user_id`), `tasks` (с `version`),
`task_history` (`changes` — JSON со старыми и новыми значениями), `task_comments`.

## Роли и права

| Действие | owner | admin | member |
|---|---|---|---|
| Видеть команду, задачи, историю, комментарии | ✅ | ✅ | ✅ |
| Создавать задачи, комментировать | ✅ | ✅ | ✅ |
| Приглашать участников (`member`) | ✅ | ✅ | ❌ |
| Приглашать администраторов (`admin`) | ✅ | ❌ | ❌ |
| Менять роли (`PATCH /teams/{id}/members/{user_id}`) | ✅ | ❌ | ❌ |
| Редактировать любую задачу | ✅ | ✅ | ❌ |
| Редактировать свою задачу (title, description, status, assignee) | ✅ | ✅ | ✅ (создатель) |
| Менять статус назначенной задачи | ✅ | ✅ | ✅ (исполнитель) |
| Аналитика `/teams/{id}/stats` | ✅ | ✅ | ❌ |

- Роль `owner` нельзя выдать приглашением или сменой роли; роль создателя изменить нельзя.
- Исполнитель должен быть участником команды задачи.
- Задачи чужих команд для пользователя «не существуют» (`404`), список задач чужой команды — `403`.
- Правила доступа — чистые функции в `internal/domain/policy.go`, покрыты unit-тестами.

## Надёжность

- **Транзакции.** Создание команды + добавление owner, приглашение, создание/обновление задачи + запись
  истории выполняются в одной транзакции (`database.DB.WithTx`, транзакция передаётся через `context`).
- **Конкурентность.** `PUT /tasks/{id}` требует поле `version`. В транзакции строка блокируется
  `SELECT ... FOR UPDATE`, версия сверяется с переданной, `UPDATE ... WHERE version = ?` увеличивает её.
  Устаревшая версия → `409 Conflict`, клиент перечитывает задачу. История строится от заблокированного
  актуального состояния и содержит только реально изменившиеся поля.
- **Кеш.** `GET /tasks` кешируется в Redis на `TASKS_CACHE_TTL` (5 минут). Ключ включает команду, версию
  кеша команды и все фильтры (`tasks:team:{id}:v{N}:status=..:assignee=..:limit=..:offset=..`).
  Любое изменение задач команды делает `INCR tasks:team:{id}:ver` — все закешированные варианты списка
  становятся недостижимыми и истекают по TTL. Права проверяются до обращения к кешу.
- **Circuit breaker.** Все обращения к Redis идут через circuit breaker: при недоступности Redis сервис
  отвечает из MySQL без задержек на таймауты.
- **Rate limiting.** Token bucket на IP клиента, ответ `429` с `Retry-After`.
  За обратным прокси задайте `TRUSTED_PROXIES`, иначе все клиенты получат общий лимит по адресу прокси.
  Docker Desktop подменяет адрес клиента на адрес шлюза, поэтому локально лимит общий.
- **Логи.** Структурированные JSON-логи (`slog`) по каждому запросу: `request_id`, метод, путь, статус,
  длительность, `user_id`. Заголовок `X-Request-ID` прокидывается в ответ.
- **Graceful shutdown** по SIGINT/SIGTERM.

## SQL-отчёт

`GET /api/v1/teams/{team_id}/stats` — только owner/admin команды. Один запрос с CTE
(`internal/repository/stats.go`), все метрики считаются от `team_tasks` (`WHERE team_id = ?`):

- количество задач по статусам;
- топ-3 исполнителя по закрытым задачам за последние 30 дней (`JOIN users`, `GROUP BY`, `LIMIT 3`);
- количество закрытых задач и среднее время закрытия (`AVG(TIMESTAMPDIFF(SECOND, created_at, closed_at))`);
- общее количество комментариев и топ-10 задач по числу комментариев (`LEFT JOIN task_comments`, `GROUP BY`).

## Примеры запросов

```bash
API=http://localhost:8080/api/v1

# Регистрация и вход
curl -s -X POST $API/register -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"secret123","name":"Alice"}'
TOKEN=$(curl -s -X POST $API/login -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"secret123"}' | jq -r .token)
AUTH="Authorization: Bearer $TOKEN"

# Команды
curl -s -X POST $API/teams -H "$AUTH" -d '{"name":"Backend"}'
curl -s $API/teams -H "$AUTH"
curl -s -X POST $API/teams/1/invite -H "$AUTH" -d '{"email":"bob@example.com","role":"member"}'
curl -s -X PATCH $API/teams/1/members/2 -H "$AUTH" -d '{"role":"admin"}'

# Задачи
curl -s -X POST $API/tasks -H "$AUTH" \
  -d '{"team_id":1,"title":"Настроить CI","description":"GitHub Actions","assignee_id":2}'
curl -s "$API/tasks?team_id=1&status=todo&assignee_id=2&limit=20&offset=0" -H "$AUTH"
curl -s -X PUT $API/tasks/1 -H "$AUTH" -d '{"status":"in_progress","version":1}'
curl -s $API/tasks/1/history -H "$AUTH"

# Комментарии
curl -s -X POST $API/tasks/1/comments -H "$AUTH" -d '{"content":"Взял в работу"}'
curl -s $API/tasks/1/comments -H "$AUTH"

# Отчёт
curl -s $API/teams/1/stats -H "$AUTH"
```

Ошибки возвращаются в формате `{"error": "..."}` со статусами
`400` (валидация), `401`, `403`, `404`, `409` (дубликат / конкурентное изменение), `429`, `500`.

## Тесты

```bash
make test        # все тесты в dev-контейнере, включая интеграционный тест SQL-отчёта на MySQL
make test-unit   # локально, без БД (интеграционный тест пропускается)
```

- `internal/repository/stats_integration_test.go` — интеграционный тест отчёта: статусы, окно 30 дней,
  топ-3 с сортировкой, среднее время закрытия, комментарии, отсутствие утечек данных другой команды.
  Запускается, когда задан `DB_HOST`.
- `internal/domain/policy_test.go` — unit-тесты правил доступа.
- `internal/service/task_test.go` — права на редактирование, история, конфликт версий, кеш и его инвалидация.
- `internal/cache/breaker_test.go` — circuit breaker.

## Структура

```
cmd/api                 точка входа
internal/app            DI-контейнер, HTTP-сервер, graceful shutdown
internal/api            роутинг, хендлеры, middleware (JWT, rate limit, логи), OpenAPI
internal/service        бизнес-логика: auth, teams, tasks, stats
internal/domain         сущности, ошибки, правила доступа
internal/repository     MySQL-репозитории и SQL-отчёт
internal/database       подключение, транзакции, миграции
internal/cache          Redis + circuit breaker
internal/config         конфигурация из env
```

## Полезные команды

```bash
make lint
make build
make docker-logs
make docker-down
make docker-clean     # с удалением volume MySQL
```
