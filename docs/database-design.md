# Database design

## Entities

- `users` own `roadmaps`; login is unique and only its Argon2id hash is stored.
- `roadmaps` own `topics` through cascading deletes.
- `topics.parent_id` defines the tree; the application service rejects cross-roadmap parents and cycles.
- `topic_dependencies` is a separate directed graph within a roadmap; the service validates ownership, duplicates, self-edges, and DAG properties.
- `topic_progress` and `topic_notes` belong to a user/topic pair.
- `topic_progress.status` stores the user-controlled topic status (`not_started`, `in_progress`, `needs_review`, `completed`); the compatible `completed` field is synchronized with it. The migration converts legacy `completed=true` values to `completed`, and all other records to `not_started`.
- `topics.priority` stores the independent priority of a topic or task (`critical`, `important`, `optional`, `skippable`), defaulting to `important`.
- `topic_documents` (migration `002_topic_documents.sql`) stores a separate topic Markdown document with `created_at`/`updated_at`; access always verifies roadmap ownership.
- `roadmap_covers` (migration `003_roadmap_covers.sql`) stores one owner-scoped cover per roadmap; binary data is excluded from list responses and served from a dedicated protected endpoint. JPEG, PNG, GIF, and WebP are accepted up to 5 MB.
- `topic_layouts` stores coordinates and collapse state separately from domain data.
- `sessions` and `auth_tokens` store hashes and expiry/revocation metadata.
- `idempotency_operations` stores a request hash and serialized response for a batch operation.
- `audit_events` records material changes without secrets or note content.
- `learning_segments` stores actual active timer intervals. An open interval is unique within a session; pause/finish closes it and resume opens a new one. For completed legacy sessions, the migration creates a conservative segment exactly matching the previously confirmed `duration_seconds`, without adding time.

## Integrity and deletion

Deleting a roadmap cascades to its topics, cover, and dependent records. Deleting a topic cascades to its entire subtree because of the self-reference, and to all notes, documents, progress, layouts, and edges. This behavior is intentional and requires explicit UI confirmation. Checks that cannot be expressed as a local SQL constraint (same-roadmap parents/edges and cycle prevention) run inside a service transaction.

## Migrations and backups

Migrations are embedded in the backend and applied in order, transactionally, at startup. Production rollback for data-destructive changes must restore a verified backup; down SQL is never run automatically.

Daily activity and achievements are not stored as an independent source of truth: the backend recalculates them from confirmed segments in the selected IANA time zone. This prevents achievements from being awarded again after synchronization and allows correct recalculation after future source edits.
