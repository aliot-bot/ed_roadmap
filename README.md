# knowledge-map-manager

Self-hosted application for personal knowledge maps, with an MCP skill for managing them safely. Users maintain roadmaps in the browser, while a connected MCP client can read and change their structure within its granted permissions.

## Features

- Knowledge maps with nested topics, dependencies, priorities, and leaf-only progress.
- Topics and tasks, a study timer, activity history, notes, and Markdown documents with images.
- Responsive React UI: graph on wide screens and tree on mobile devices.
- Go API, PostgreSQL, restricted key login, session cookies, and CSRF protection.
- An MCP endpoint with separate read/write scopes. MCP changes structure only: it cannot mark progress or edit notes.
- Caddy and Docker Compose for deployment on one VPS, backups, and isolated restoration.

## MCP skill

[SKILL.md](SKILL.md) contains the portable `knowledge-map-manager` skill. It treats MCP tools as the source of truth, asks for confirmation when a goal is ambiguous, and never invents identifiers, progress, or learning data.

Supported MCP tools:

- read: `list_roadmaps`, `get_roadmap`, `get_learning_statistics`, `get_learning_activity`;
- structure changes: `create_roadmap`, `create_topics_batch`, `update_topic`, `set_topic_priority`, `set_dependencies_batch`, `add_checklist_items`.

## Quick start

Docker Engine and Docker Compose are required.

1. Copy `.env.example` to `.env`, replace every `replace-with-...` value, and set `DOMAIN`. In production, `.env` must have `0600` permissions.
2. Start the services:

   ```sh
   docker compose up -d --build
   docker compose ps
   ```

3. On a new installation, create an owner and store the generated key:

   ```sh
   docker compose exec backend /roadmap-api users create owner "Owner"
   ```

4. Open `https://<DOMAIN>` and sign in with the key. PostgreSQL and the backend do not publish host ports; only 80/443 are publicly available through Caddy.

## Development

```sh
cd backend && go test ./... && go vet ./...
cd ../frontend && npm ci && npm run lint && npm test && npm run build
```

Before a release, also run:

```sh
docker compose --env-file .env config --quiet
sh -n scripts/backup-postgres.sh scripts/restore-postgres.sh
```

## Documentation

- [Architecture](docs/architecture.md)
- [API and MCP](docs/api-specification.md)
- [OpenAPI](docs/openapi.yaml)
- [Data model](docs/database-design.md)
- [Decisions](docs/decisions.md)
- [Deployment and recovery](docs/deployment.md)
- [Security audit](docs/security-audit.md)

## Layout

- `SKILL.md` — the `knowledge-map-manager` MCP skill;
- `AGENTS.md` — rules for changing the application;
- `backend/` — Go API, MCP adapter, and migrations;
- `frontend/` — React client;
- `docs/` — technical and operational documentation;
- `scripts/` — backup and isolated restore scripts.

## Important limitations

- The static bearer token is intended for a trusted MCP client and is not an OAuth 2.1/PKCE integration.
- Public production deployment requires DNS, a VPS, a firewall, and real TLS checks.
- Before running multiple instances, strengthen serialization of concurrent hierarchy operations and MCP idempotency.
