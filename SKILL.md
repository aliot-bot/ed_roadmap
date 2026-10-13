---
name: knowledge-map-manager
description: "Manage Knowledge Map learning roadmaps: add and reorganize topics, maintain checklists and priorities, and provide evidence-based progress and learning analytics. Use for requests about knowledge maps, roadmaps, learning topics, or study progress; not for project plans that are not stored in Knowledge Map."
---

# Knowledge Map Manager

Use the `knowledge_map` MCP tools as the source of truth. Keep the user's learning structure intact and make only the requested changes.

## Find and confirm the target

- For a request that names a roadmap, call `list_roadmaps` and match it by title. If there is no exact match, say that it was not found; do not silently create a replacement.
- If there is only one relevant roadmap or the user clearly refers to the current one, use it. Otherwise ask which map to change.
- Before structural edits, call `get_roadmap` when you need topic IDs or need to prevent duplicate topics.

## Add and edit content

- Create a roadmap only when the user explicitly asks for a new map. Use `create_roadmap` with an idempotency key.
- Add topics with `create_topics_batch`. Preserve the requested hierarchy with `parent_id` or `parent_ref`, ordering with `sort_order`, and priority at creation time.
- Translate priorities unless the user specifies another mapping: critical/«критично»/🔴 → `critical`; important/«важно»/🟠 → `important`; optional/«опционально» → `optional`; skippable/«пропустить» → `skippable`.
- For an existing topic use `update_topic`; to change only its priority, use `set_topic_priority`. Never overwrite unrelated title, parent, description, or ordering fields.
- Put requested learning tasks into the topic's actual checklist using `add_checklist_items`. Do not substitute checklist items with a topic description. This API adds unchecked items, so do not use it to replace or remove existing checklist items.
- Use a distinct, stable idempotency key for every write. After a batch write, verify the affected map with `get_roadmap` when the returned result does not already prove success.

## Analytics and reporting

- For roadmap progress, use `get_roadmap`; report its supplied counts and percentages rather than estimating them.
- For time and activity questions, use `get_learning_statistics` and/or `get_learning_activity`. Use the user's requested period and timezone; otherwise state the period/timezone used by the tool.
- Clearly distinguish confirmed data from an inference. If the requested metric is unavailable or the map cannot be found, say «не нашёл» and name what is missing.
- Keep reports compact: lead with progress, then the most useful blockers, priorities, or next topics. Do not mark work complete or modify learning data while only reporting analytics.

## Safety and communication

- Treat adding, moving, renaming, and reprioritizing as changes. Do them only when the user asks; analytics and lookup requests are read-only.
- Do not invent IDs, completion states, durations, dependencies, or checklist results. Ask one concise question if an unresolved choice materially affects a write.
