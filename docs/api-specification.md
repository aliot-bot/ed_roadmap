# API specification

All REST routes, except health and bootstrap before an owner exists, require a browser session. Cookie-authenticated mutations require `X-CSRF-Token`. JSON limits apply before decoding.

## Auth

- `POST /api/auth/bootstrap` `{login,password}` — creates the only initial owner; subsequent calls return `409`.
- `POST /api/auth/login` `{login,password}` — issues an HttpOnly session cookie and a CSRF response header.
- `GET /api/auth/me` — returns `{id,login}` and the current CSRF header.
- `POST /api/auth/logout` — revokes the current session.

## Roadmaps and topics

- `GET|POST /api/roadmaps`
- `GET|PATCH|DELETE /api/roadmaps/{id}`
- `GET /api/roadmaps/{id}/cover` returns the private image bytes; `PUT` replaces it with a multipart `file` (JPEG, PNG, GIF or WebP, up to 5 MB); `DELETE` removes it. PUT/DELETE require CSRF.
- `GET|POST /api/roadmaps/{id}/topics`
- `PATCH|DELETE /api/topics/{id}`
- `PUT /api/topics/{id}/completion` `{completed:boolean}`
- `GET|PUT /api/topics/{id}/note` / `{content:string}`
- `GET|PUT /api/topics/{id}/document` / `{content:string}` — a separate Markdown document, up to 200,000 Unicode characters; the response includes `created_at` and `updated_at` after saving.
- `GET|PUT /api/roadmaps/{id}/dependencies` / `{dependencies:[...]}`
- `GET /api/roadmaps/{id}/progress`
- `GET /api/analytics` — owner-scoped roadmap count and leaf-only statistics across all roadmaps, broken down by roadmap; `percent` is `null` when there are no leaf topics.
- `GET|PUT /api/roadmaps/{id}/layout` / `{items:[{topic_id,x,y,collapsed}]}`
- `POST /api/auth/key-login` `{key:string}` — restricted login; an error response does not reveal the user. Public bootstrap is disabled by default.
- Topics accept `kind: topic|task`, `priority: critical|important|optional|skippable` and task-only `task_status: not_started|in_progress|completed`.
- `PUT /api/topics/{id}/status` accepts topic-only `not_started|in_progress|needs_review|completed`. It is owner-scoped, rejects tasks and non-leaf aggregate sections, and keeps legacy `completed` compatibility in sync.
- `PATCH /api/topics/{id}` can update `priority`; topic status may also be supplied but remains separate from task status.
- `POST /api/learning/sessions/start` `{elementId}`; `POST /api/learning/sessions/{id}/pause|resume|finish`; `GET /api/learning/sessions/active`; `GET /api/learning/sessions`.
- `GET /api/learning/overview` and `GET /api/learning/activity?from=...&to=...&timezone=...` return confirmed timer aggregates. Dates may be RFC3339 or `YYYY-MM-DD`; ranges are half-open. Activity items include `study_minutes`, `intensity` and cumulative daily achievement codes.
- `GET /api/learning/activity/{date}?timezone=...` returns one local-day aggregate or `day: null`.
- `GET /api/learning/achievements` returns all-time 4h/6h/8h day counts by default; an explicit range limits the calculation.
- `POST /api/uploads/images` is multipart `topic_id` + `file`; `GET /api/uploads/images/{id}` is session- and owner-protected.
- `GET /api/learning/analysis/latest`; `POST /api/learning/analysis`. Without a configured provider the stored report has `status: unavailable` and empty Markdown.

Responses use stable snake_case fields represented in `frontend/src/types.ts`. Lists are either JSON arrays or `{items:[...]}`; the current client accepts both. Errors use `{error|message, details?}` and an appropriate `400/401/403/404/409/429` status.

The machine-readable REST contract is `docs/openapi.yaml`.

## MCP

`POST /mcp` implements JSON-RPC MCP Streamable HTTP and requires `Authorization: Bearer ...`. Supported structural tools:

- read: `list_roadmaps`, `get_roadmap`, `get_learning_statistics`, `get_learning_activity`;
- write: `create_roadmap`, `create_topics_batch`, `update_topic`, `set_topic_priority`, `set_dependencies_batch`, `add_checklist_items`.

Write tools require write scope and never accept completion/note fields. `add_checklist_items` accepts a topic and 1–80 item texts, appends them as unchecked, and preserves existing checklist items. Batch writes are bounded by `MCP_MAX_BATCH_TOPICS`; idempotent calls use an operation key. No delete tools are exposed in MVP. `GET /mcp` may advertise/maintain transport behavior but does not bypass authorization.

`get_roadmap` publishes topic `status` and `priority`; `set_topic_priority` accepts `critical`, `important`, `optional` or `skippable`. Analytics remains read-only: these fields are observable signals, not permissions to mutate personal progress.

## Compatibility boundary

Static bearer mode is a trusted-client integration, not proof of ChatGPT remote OAuth compatibility. OAuth discovery, authorization server metadata, PKCE and dynamic/client registration require a separately selected authorization design and real-client test.
