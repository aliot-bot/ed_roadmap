# Deployment and operations

## VPS prerequisites

- Linux VPS with Docker Engine/Compose and firewall;
- A/AAAA record for `DOMAIN` pointing at the VPS;
- inbound TCP 80/443 and UDP 443; PostgreSQL 5432 remains closed;
- SSH keys and an unprivileged operator account.

## First launch

1. Copy the repository to the VPS.
2. Copy `.env.example` to `.env`, replace every placeholder with a unique secret, and run `chmod 600 .env`.
3. Validate with `docker compose config --quiet`.
4. Start with `docker compose up -d --build`.
5. Wait for `docker compose ps` to show healthy services. Inspect `docker compose logs backend caddy` on failure.
6. Open HTTPS and complete the UI first-launch flow once. Store the owner password outside the repository.
7. Test unauthenticated `/api/auth/me` and `/mcp`, then the authenticated UI and MCP with their separate credentials.

Migrations are embedded in the backend, ordered, and applied transactionally before the server starts listening.

## Update

1. Create and verify a backup with `./scripts/backup-postgres.sh`.
2. Review changed `.env.example`, migrations and release notes.
3. Pull/copy the reviewed release.
4. Run `docker compose build` and `docker compose up -d`.
5. Check health/logs and execute login, map read/write, note/progress and MCP smoke paths.

## Backup and restore

`backup-postgres.sh` writes a private custom-format archive through a temporary file, verifies its table of contents and atomically renames it. Copy backups off the VPS and define retention outside this repository.

Always verify restore into a separate database:

```sh
RESTORE_DATABASE=knowledge_map_restore_check ./scripts/restore-postgres.sh backups/knowledge-map-<timestamp>.dump
```

Replacing an existing or configured database requires explicit confirmation variables printed by the script. Do not use them until the target name and backup have been independently checked.

## Rollback

Application-only rollback: deploy the previous image/source if its schema is forward-compatible. For an incompatible/data-changing migration, stop writes and restore a pre-update backup into a separately named database first. Automatic down migrations are not part of the production path.

## Move to another VPS

Transfer the repository/release, `.env` through a secure channel, and a verified dump. Start PostgreSQL on the target, restore into its configured database, then start backend/frontend/Caddy and update DNS. Keep the old VPS read-only until HTTPS and data checks pass.

## Health and troubleshooting

- backend container health invokes `/roadmap-api healthcheck`, which requests local `/healthz` without a shell;
- frontend health requests nginx `/healthz`;
- PostgreSQL uses `pg_isready`;
- Caddy health validates its binary while public behavior is checked from outside.

Common failures:

- Caddy certificate pending: verify DNS, firewall, clock and Caddy logs.
- Backend unhealthy: inspect DB health, `DATABASE_URL` construction and migration error logs.
- Browser mutation `403`: refresh `/api/auth/me`; the client recovers its session-bound CSRF token.
- MCP `401`: verify exact bearer value and that `MCP_ENABLED=true`.
- MCP `403`: verify owner exists and `MCP_SCOPES` contains the requested read/write scope.
