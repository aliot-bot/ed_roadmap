# Architecture

## Context

The system is a modular monolith: a React SPA and one stateless Go process that exposes REST and MCP through a shared application/service layer. PostgreSQL is the single source of truth. Caddy terminates TLS and routes public traffic; PostgreSQL and the backend do not publish host ports.

```text
Browser -> HTTPS/Caddy -> React UI
                     \-> REST adapter ----\
MCP client -> HTTPS/Caddy -> MCP adapter ---+-> application services -> repositories -> PostgreSQL
```

## Boundaries

- `domain`: entities, error types, and invariants; it does not depend on transport or the database.
- `service`: use cases, ownership, transactional rules, DAG/hierarchy, and progress.
- `repository`: parameterized SQL through pgx, transactions, and persistent sessions/tokens.
- `httpapi`: DTOs, cookies/CSRF, authorization, limits, and safe error mapping.
- `mcp`: JSON-RPC Streamable HTTP and tool schemas; it calls the same services, never SQL directly.
- `frontend`: server state through TanStack Query; graph and tree are two views of the same data.

## Key decisions

- UUIDs are stable external identifiers.
- The hierarchy (`parent_id`) and prerequisite DAG (`topic_dependencies`) are stored separately.
- Progress is calculated from leaves only. A group cannot be checked independently.
- Layout and collapse state are separate from topic content and progress.
- A roadmap cover is separate from roadmap lists and is loaded through an owner-scoped endpoint; a Markdown document opens in its own dialog, without overloading the topic card.
- The session cookie stores a random opaque token; only its hash is stored in the database. Cookie mutations require a CSRF token.
- MCP bearer tokens have separate read/write scopes. Tools cannot delete, edit notes, or mutate progress.
- Batch MCP operations are bounded, transactional, and support an operation/idempotency key.

## Error flow

Domain/service return classified validation, not-found, conflict, and forbidden errors. Adapters map them to stable HTTP/JSON-RPC responses without SQL, stack traces, or secrets. The client retains entered values after recoverable errors.

## Performance

Indexes cover owner roadmaps, roadmap hierarchy, dependencies, and session expiry. Loading a roadmap aggregates a bounded number of queries, and the UI hides collapsed subtrees. Redis, queues, and microservices are not added for the MVP.
