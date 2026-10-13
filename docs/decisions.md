# Architecture decisions

## ADR-001 — Modular monolith

Use one Go backend with REST and MCP adapters over shared services. Rejected microservices, Redis, Kafka and Kubernetes because the single-owner/single-VPS scope does not justify them.

## ADR-002 — Separate structure, progress and layout

Topics/hierarchy and prerequisite edges describe knowledge structure; per-user completion/notes describe personal state; layout describes presentation. Separation prevents moving/renaming a node from resetting progress.

## ADR-003 — Leaf-only progress

Only leaf topics count. Group progress is derived from leaf descendants and group completion does not cascade, matching the source PRD.

## ADR-004 — Persistent opaque browser sessions

Use random session cookies whose hashes and expiry live in PostgreSQL, with HttpOnly/Secure/SameSite and CSRF for mutations. Stateless signed user claims were rejected because revocation and session visibility are explicit requirements.

## ADR-005 — Trusted MCP mode first, OAuth boundary explicit

Implement scoped, revocable static bearer tokens for a limited trusted client and no destructive/progress/note tools. Do not advertise this as universal remote ChatGPT compatibility. OAuth 2.1/PKCE requires a concrete provider/client decision and external test.

## ADR-006 — Dual graph/tree interaction

React Flow is the primary desktop visualization; the semantic hierarchical tree is a first-class alternative for narrow screens, touch and keyboard use. Drag-and-drop remains optional.

## ADR-007 — Forward embedded migrations

Backend applies ordered embedded SQL in transactions. Production automatic down migrations are avoided; destructive rollback uses a verified backup.

## ADR-008 — Topic status and element priority are independent

Leaf topics use the owner-controlled states `not_started`, `in_progress`, `needs_review` and `completed`. Tasks keep their separate three-state `task_status`. Every topic or task has an independent `critical`, `important`, `optional` or `skippable` priority. Only completed leaf topics contribute to the familiar roadmap percentage; task leaves are excluded from that denominator and are reported separately through their own status.

## ADR-009 — Confirmed active segments are the activity source

Learning activity is derived from closed active intervals, not page views or timer starts. Start/pause/resume/finish update the session and its segment in one transaction. Aggregation intersects each segment with local calendar-day boundaries, sums seconds per day, then converts to minutes. The 4/6/8-hour achievements are deterministic, cumulative derived views rather than append-only awards, so corrected source data can be recalculated without duplicates.

## ADR-010 — Keep reordering, but move it out of the primary action row

The former “Move up” and “Move down” actions swap `sort_order` with the adjacent sibling. They persist in PostgreSQL, affect tree/default graph ordering, do not change hierarchy, prerequisite edges, or priority, and are disabled at list boundaries. This is the only explicit keyboard/touch ordering path, so it is retained under “Order in structure” and named “Move up/down in the list” instead of being removed.
