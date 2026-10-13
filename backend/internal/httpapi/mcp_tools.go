package httpapi

func mcpTools() []map[string]any {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           properties,
			"required":             required,
		}
	}
	uuidSchema := map[string]any{
		"type":   "string",
		"format": "uuid",
	}
	idempotency := map[string]any{
		"type":      "string",
		"minLength": 8,
		"maxLength": 200,
	}
	priority := map[string]any{
		"type": "string",
		"enum": []string{"critical", "important", "optional", "skippable"},
	}
	topic := object(
		map[string]any{
			"client_ref": map[string]any{
				"type":      "string",
				"maxLength": 100,
			},
			"parent_id": uuidSchema,
			"parent_ref": map[string]any{
				"type":      "string",
				"maxLength": 100,
			},
			"title": map[string]any{
				"type":      "string",
				"minLength": 1,
				"maxLength": 300,
			},
			"description": map[string]any{
				"type":      "string",
				"maxLength": 20000,
			},
			"sort_order": map[string]any{
				"type": "integer",
			},
			"priority": priority,
		},
		"title",
	)
	topicOutput := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":         uuidSchema,
			"roadmap_id": uuidSchema,
			"title": map[string]any{
				"type": "string",
			},
			"status": map[string]any{
				"type": "string",
				"enum": []string{"not_started", "in_progress", "needs_review", "completed"},
			},
			"priority": priority,
			"kind": map[string]any{
				"type": "string",
			},
		},
	}
	dependency := object(
		map[string]any{
			"predecessor_topic_id": uuidSchema,
			"successor_topic_id":   uuidSchema,
		},
		"predecessor_topic_id",
		"successor_topic_id",
	)
	checklistItem := object(
		map[string]any{
			"text": map[string]any{
				"type":      "string",
				"minLength": 1,
				"maxLength": 500,
			},
		},
		"text",
	)

	return []map[string]any{
		{
			"name":        "list_roadmaps",
			"description": "List owned roadmaps",
			"inputSchema": object(map[string]any{}),
		},
		{
			"name":        "get_roadmap",
			"description": "Get roadmap metadata, topics, dependencies and progress",
			"inputSchema": object(
				map[string]any{"roadmap_id": uuidSchema},
				"roadmap_id",
			),
			"outputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"topics": map[string]any{
						"type":  "array",
						"items": topicOutput,
					},
				},
			},
		},
		{
			"name":        "get_learning_statistics",
			"description": "Read confirmed timer aggregates; unsupported metrics are null",
			"inputSchema": object(map[string]any{
				"from": map[string]any{
					"type": "string",
				},
				"to": map[string]any{
					"type": "string",
				},
				"timezone": map[string]any{
					"type": "string",
				},
			}),
		},
		{
			"name":        "get_learning_activity",
			"description": "Read confirmed timer activity by local day",
			"inputSchema": object(map[string]any{
				"from": map[string]any{
					"type": "string",
				},
				"to": map[string]any{
					"type": "string",
				},
				"timezone": map[string]any{
					"type": "string",
				},
			}),
		},
		{
			"name":        "create_roadmap",
			"description": "Create a roadmap; requires idempotency_key",
			"inputSchema": object(
				map[string]any{
					"idempotency_key": idempotency,
					"title": map[string]any{
						"type":      "string",
						"minLength": 1,
						"maxLength": 200,
					},
					"description": map[string]any{
						"type":      "string",
						"maxLength": 5000,
					},
				},
				"idempotency_key",
				"title",
			),
		},
		{
			"name":        "create_topics_batch",
			"description": "Atomically create 1-200 topics; requires idempotency_key",
			"inputSchema": object(
				map[string]any{
					"idempotency_key": idempotency,
					"roadmap_id":      uuidSchema,
					"topics": map[string]any{
						"type":     "array",
						"minItems": 1,
						"maxItems": 200,
						"items":    topic,
					},
				},
				"idempotency_key",
				"roadmap_id",
				"topics",
			),
		},
		{
			"name":        "update_topic",
			"description": "Update structural topic fields; requires idempotency_key",
			"inputSchema": object(
				map[string]any{
					"idempotency_key": idempotency,
					"topic_id":        uuidSchema,
					"title": map[string]any{
						"type":      "string",
						"minLength": 1,
						"maxLength": 300,
					},
					"description": map[string]any{
						"type":      "string",
						"maxLength": 20000,
					},
					"parent_id": map[string]any{
						"anyOf": []any{
							uuidSchema,
							map[string]any{"type": "null"},
						},
					},
					"sort_order": map[string]any{
						"type": "integer",
					},
				},
				"idempotency_key",
				"topic_id",
			),
		},
		{
			"name":        "set_topic_priority",
			"description": "Set a topic or task priority; requires idempotency_key",
			"inputSchema": object(
				map[string]any{
					"idempotency_key": idempotency,
					"topic_id":        uuidSchema,
					"priority":        priority,
				},
				"idempotency_key",
				"topic_id",
				"priority",
			),
		},
		{
			"name":        "set_dependencies_batch",
			"description": "Atomically replace dependencies; requires idempotency_key",
			"inputSchema": object(
				map[string]any{
					"idempotency_key": idempotency,
					"roadmap_id":      uuidSchema,
					"dependencies": map[string]any{
						"type":     "array",
						"maxItems": 4000,
						"items":    dependency,
					},
				},
				"idempotency_key",
				"roadmap_id",
				"dependencies",
			),
		},
		{
			"name":        "add_checklist_items",
			"description": "Add unchecked items to a topic checklist without replacing existing items; requires idempotency_key",
			"inputSchema": object(
				map[string]any{
					"idempotency_key": idempotency,
					"topic_id":        uuidSchema,
					"items": map[string]any{
						"type":     "array",
						"minItems": 1,
						"maxItems": 80,
						"items":    checklistItem,
					},
				},
				"idempotency_key",
				"topic_id",
				"items",
			),
		},
	}
}
