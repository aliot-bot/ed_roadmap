# Проектирование базы данных

## Сущности

- `users` владеют `roadmaps`; login уникален, хранится только Argon2id hash.
- `roadmaps` каскадно владеют `topics`.
- `topics.parent_id` задаёт дерево; application service запрещает cross-roadmap parent и циклы.
- `topic_dependencies` — отдельный направленный граф внутри roadmap; service проверяет принадлежность, дубликаты, self-edge и DAG.
- `topic_progress` и `topic_notes` принадлежат паре user/topic.
- `topic_progress.status` хранит ручной статус темы (`not_started`, `in_progress`, `needs_review`, `completed`); совместимый `completed` синхронизирован с ним. Миграция переводит старые `completed=true` в `completed`, остальные записи — в `not_started`.
- `topics.priority` хранит независимый приоритет темы или задания (`critical`, `important`, `optional`, `skippable`), по умолчанию `important`.
- `topic_documents` (миграция `002_topic_documents.sql`) хранит отдельный Markdown-конспект темы с `created_at`/`updated_at`; доступ всегда проверяет владельца roadmap.
- `roadmap_covers` (миграция `003_roadmap_covers.sql`) хранит одну owner-scoped обложку на roadmap; бинарные данные не попадают в списочные ответы, а выдаются отдельным защищённым endpoint. Допустимы JPEG, PNG, GIF и WebP до 5 МБ.
- `topic_layouts` хранит координаты и collapsed отдельно от предметных данных.
- `sessions` и `auth_tokens` хранят hashes и сроки/revocation metadata.
- `idempotency_operations` хранит hash запроса и сериализованный ответ пакетной операции.
- `audit_events` фиксирует существенные изменения без секретов и текста заметок.
- `learning_segments` хранит фактические активные интервалы таймера. Открытый интервал уникален в пределах session; pause/finish закрывает его, resume открывает новый. Для старых завершённых sessions миграция создаёт консервативный сегмент ровно на ранее подтверждённую `duration_seconds`, не добавляя времени.

## Целостность и удаление

Удаление roadmap каскадно удаляет его темы, обложку и зависимые записи. Удаление topic каскадно удаляет всё поддерево из-за self-reference и все notes/documents/progress/layout/edges. Это поведение намеренное и требует явного подтверждения UI. Проверки, которые нельзя выразить локальным SQL constraint (same-roadmap parent/edge и отсутствие циклов), выполняются внутри транзакции service.

## Миграции и резервное копирование

Миграции встроены в backend и применяются по порядку в транзакциях при старте. Production rollback для data-destructive изменений должен выполняться восстановлением из проверенной резервной копии; down SQL не запускается автоматически.

Дневная активность и достижения не хранятся как независимая истина: backend пересчитывает их из подтверждённых сегментов в выбранной IANA timezone. Это исключает повторную выдачу достижения после синхронизации и позволяет корректно пересчитать результат после будущего редактирования источника.
