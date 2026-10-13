# Security audit

Updated: 2026-10-11. Scope: repository configuration and the local Compose integration environment; no production penetration test was performed.

## Implemented controls

- **Passwords/sessions:** Argon2id password hashes; random opaque session and CSRF values; only hashes persisted; HttpOnly/Secure/SameSite session cookie; session-bound CSRF header for browser mutations; logout deletes the server session.
- **Authorization/IDOR:** every repository operation accepts owner ID; REST session and MCP principal are resolved before services; MCP separates read/write scopes.
- **Input/SQL:** bounded bodies and batch counts, strict JSON/MCP arguments, parameterized pgx SQL, text limits, UUID parsing, hierarchy/dependency validation.
- **Data integrity:** PostgreSQL foreign keys and cascading ownership; batch topic/dependency/layout transactions; leaf-only progress calculation; stable UUIDs.
- **MCP:** bearer required; destructive, note and completion tools absent; schemas published; structural writes use operation keys and audit metadata.
- **Edge/runtime:** only Caddy publishes ports; PostgreSQL and application networks are private; TLS/security headers, no-new-privileges, dropped backend capabilities, read-only backend filesystem, log rotation and graceful shutdown.
- **Recovery:** backup archive is verified before atomic publication; restore validates before any destructive choice and defaults to an isolated database.
- **Dependencies:** production/full npm audit reported zero findings after upgrading Vitest; ESLint is on supported major 10. `govulncheck` initially found vulnerable Go 1.26.5, chi, pgx and x/text paths; dependencies and the container builder were upgraded, then race tests, vet, build and `govulncheck` completed successfully on Go 1.26.9.

## Threat review

| Risk | Status | Evidence/remaining work |
|---|---|---|
| SQL injection | PASS | parameterized repository queries; no arbitrary MCP SQL |
| XSS | PASS with browser follow-up | React text rendering, no `dangerouslySetInnerHTML`; CSP at edge |
| CSRF | PASS local integration | mutation rejected without token; restored session mutation passed |
| IDOR/access control | PASS design/local smoke | owner-filtered queries and unauthorized API/MCP 401; broader PostgreSQL integration suite desirable |
| Password brute force | PASS with limitation | login/bootstrap limiter; proxy-aware behavior must remain tested when edge topology changes |
| Secret leakage | PASS repository review | `.env` ignored, example placeholders only, MCP token not logged |
| MCP overreach | PASS trusted mode | scoped structure tools only; no progress/notes/delete tools |
| Dependency cycles | PASS functional tests | DAG validation; concurrency serialization hardening remains below |
| Backup loss | PASS local exercise | pg_dump and isolated restore executed in the test Compose environment |

## Residual findings

- **High before multi-instance/high-contention use:** operation-key idempotency and hierarchy validation need stronger database-level serialization to eliminate narrow concurrent race windows and crash-between-write-and-idempotency-record behavior.
- **Medium:** static MCP bearer is a shared env secret; per-token expiry/revocation and OAuth 2.1/PKCE are not implemented. Rotate by changing the secret and recreating backend.
- **Medium:** external remote-client compatibility, production TLS/DNS, firewall and host SSH hardening were not tested.
- **Medium:** full browser security/accessibility pass in Firefox/WebKit/mobile devices is outstanding.
- **Low:** the current browser/UI acceptance used Chromium only; repeat the security/accessibility paths in Firefox and WebKit before public release.

No known secret is committed. The project must remain `PARTIALLY_READY` while mandatory production/external-client/visual checks are outstanding.
