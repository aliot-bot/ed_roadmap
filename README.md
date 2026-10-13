# knowledge-map-manager

Self-hosted приложение для личных карт знаний и MCP-навык для безопасного управления ими. Пользователь ведёт дорожные карты в браузере, а подключённый MCP-клиент может читать и менять их структуру в пределах выданных прав.

## Возможности

- Карты знаний с вложенными темами, зависимостями, приоритетами и листовым прогрессом.
- Темы и задания, учебный таймер, история активности, заметки и Markdown-конспекты с изображениями.
- Адаптивный React-интерфейс: граф на широком экране и дерево на мобильных устройствах.
- Go API, PostgreSQL, закрытый вход по ключу, session cookie и CSRF-защита.
- MCP endpoint с отдельными read/write scopes. MCP меняет только структуру: не отмечает прогресс и не редактирует заметки.
- Caddy и Docker Compose для развёртывания на одном VPS, резервного копирования и изолированного восстановления.

## MCP-навык

[SKILL.md](SKILL.md) содержит переносимый навык `knowledge-map-manager`. Он использует MCP-инструменты как источник истины, запрашивает подтверждение неоднозначной цели и не выдумывает идентификаторы, прогресс или учебные данные.

Поддерживаемые MCP-инструменты:

- чтение: `list_roadmaps`, `get_roadmap`, `get_learning_statistics`, `get_learning_activity`;
- изменение структуры: `create_roadmap`, `create_topics_batch`, `update_topic`, `set_topic_priority`, `set_dependencies_batch`, `add_checklist_items`.

## Быстрый запуск

Нужны Docker Engine и Docker Compose.

1. Скопируйте `.env.example` в `.env`, замените все значения `replace-with-...` и задайте `DOMAIN`. Для production у `.env` должны быть права `0600`.
2. Запустите сервисы:

   ```sh
   docker compose up -d --build
   docker compose ps
   ```

3. На новой установке создайте владельца и сохраните выданный ключ:

   ```sh
   docker compose exec backend /roadmap-api users create owner "Owner"
   ```

4. Откройте `https://<DOMAIN>` и войдите ключом. PostgreSQL и backend не публикуют порты на хост; наружу доступны только 80/443 через Caddy.

## Разработка

```sh
cd backend && go test ./... && go vet ./...
cd ../frontend && npm ci && npm run lint && npm test && npm run build
```

Перед релизом также выполните:

```sh
docker compose --env-file .env config --quiet
sh -n scripts/backup-postgres.sh scripts/restore-postgres.sh
```

## Документация

- [Архитектура](docs/architecture.md)
- [API и MCP](docs/api-specification.md)
- [OpenAPI](docs/openapi.yaml)
- [Модель данных](docs/database-design.md)
- [Решения](docs/decisions.md)
- [Развёртывание и восстановление](docs/deployment.md)
- [Аудит безопасности](docs/security-audit.md)

## Структура

- `SKILL.md` — MCP-навык `knowledge-map-manager`;
- `AGENTS.md` — правила внесения изменений в приложение;
- `backend/` — Go API, MCP-адаптер и миграции;
- `frontend/` — React-клиент;
- `docs/` — техническая и эксплуатационная документация;
- `scripts/` — backup и изолированное restore.

## Важные ограничения

- Статический bearer-токен предназначен для доверенного MCP-клиента и не является OAuth 2.1/PKCE-интеграцией.
- Для публичного production-развёртывания нужны DNS, VPS, firewall и реальные проверки TLS.
- Перед использованием в нескольких инстансах требуется усилить сериализацию конкурентных операций с иерархией и MCP idempotency.
