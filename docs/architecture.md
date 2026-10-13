# Архитектура

## Контекст

Система — модульный монолит: React SPA и один stateless Go-процесс, который предоставляет REST и MCP поверх общего application/service слоя. PostgreSQL — единственный источник истины. Caddy завершает TLS и маршрутизирует публичный трафик; PostgreSQL и backend не публикуют хостовые порты.

```text
Browser -> HTTPS/Caddy -> React UI
                     \-> REST adapter ----\
MCP client -> HTTPS/Caddy -> MCP adapter ---+-> application services -> repositories -> PostgreSQL
```

## Границы

- `domain`: сущности, типы ошибок и инварианты, не зависит от транспорта/БД.
- `service`: use cases, ownership, транзакционные правила, DAG/иерархия, прогресс.
- `repository`: параметризованный SQL через pgx, транзакции и persistent sessions/tokens.
- `httpapi`: DTO, cookie/CSRF, авторизация, limits, безопасное отображение ошибок.
- `mcp`: JSON-RPC Streamable HTTP и схемы tools; вызывает те же services, не SQL.
- `frontend`: server-state через TanStack Query; graph/tree — два представления одних данных.

## Ключевые решения

- UUID являются устойчивыми внешними идентификаторами.
- Иерархия (`parent_id`) и prerequisite DAG (`topic_dependencies`) хранятся отдельно.
- Прогресс считается только по листьям. Группа не получает самостоятельную галочку.
- Layout/collapse отделены от topic content и progress.
- Обложка roadmap хранится отдельно от списка карт и загружается owner-scoped endpoint; Markdown-конспект открывается отдельным диалогом, не перегружая карточку темы.
- Session cookie хранит случайный непрозрачный токен; в БД — только hash. Изменяющие cookie-запросы требуют CSRF token.
- MCP bearer tokens имеют отдельные read/write scopes. Tools не предоставляют удаление, изменение заметок или progress.
- Batch MCP операции ограничены, транзакционны и поддерживают operation/idempotency key.

## Потоки ошибок

Domain/service возвращают классифицированные validation/not-found/conflict/forbidden ошибки. Адаптеры переводят их в стабильные HTTP/JSON-RPC ответы без SQL, stack traces и секретов. Клиент сохраняет введённые значения при recoverable ошибке.

## Производительность

Индексы покрывают owner roadmaps, roadmap hierarchy, dependencies и session expiry. Получение карты агрегирует ограниченное число запросов, а UI скрывает свёрнутые поддеревья. Для MVP не добавляются Redis, queues или микросервисы.
