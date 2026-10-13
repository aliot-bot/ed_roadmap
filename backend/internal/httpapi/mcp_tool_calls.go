package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"roadmap/backend/internal/apperrors"
	"roadmap/backend/internal/domain"
	"roadmap/backend/internal/service"
)

func (a *API) callTool(w http.ResponseWriter, r *http.Request, id any, raw json.RawMessage) {
	var c struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	if err := json.Unmarshal(raw, &c); err != nil {
		rpcError(w, id, -32602, "invalid tool call")
		return
	}

	p := principal(r)
	var out any
	var err error

	switch c.Name {
	case "list_roadmaps":
		var x struct{}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil {
			out, err = a.app.ListRoadmaps(r.Context(), p)
		}

	case "get_roadmap":
		var x struct {
			RoadmapID uuid.UUID `json:"roadmap_id"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.RoadmapID == uuid.Nil {
			err = domain.ValidationError{Field: "roadmap_id", Reason: "is required"}
		}
		if err == nil {
			out, err = a.roadmapDetail(r.Context(), p, x.RoadmapID)
		}

	case "get_learning_statistics", "get_learning_activity":
		var x struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Timezone string `json:"timezone"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil {
			from, to := time.Now().UTC().Add(-30*24*time.Hour), time.Now().UTC()
			if x.From != "" {
				from, err = time.Parse(time.RFC3339, x.From)
			}
			if err == nil && x.To != "" {
				to, err = time.Parse(time.RFC3339, x.To)
			}
			loc := time.UTC
			if err == nil && x.Timezone != "" {
				loc, err = time.LoadLocation(x.Timezone)
			}
			if err != nil {
				err = domain.ValidationError{Field: "range", Reason: "invalid RFC3339 range or timezone"}
			} else if c.Name == "get_learning_statistics" {
				out, err = a.app.LearningOverview(r.Context(), p, from, to, loc)
			} else {
				out, err = a.app.LearningActivity(r.Context(), p, from, to, loc)
			}
		}

	case "create_roadmap":
		var x struct {
			IdempotencyKey string `json:"idempotency_key"`
			Title          string `json:"title"`
			Description    string `json:"description"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil {
			out, err = a.app.Idempotent(
				r.Context(),
				p,
				x.IdempotencyKey,
				x,
				func() (any, error) {
					return a.app.CreateRoadmap(r.Context(), p, x.Title, x.Description)
				},
			)
		}

	case "create_topics_batch":
		var x struct {
			IdempotencyKey string              `json:"idempotency_key"`
			RoadmapID      uuid.UUID           `json:"roadmap_id"`
			Topics         []domain.TopicInput `json:"topics"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.RoadmapID == uuid.Nil {
			err = domain.ValidationError{Field: "roadmap_id", Reason: "is required"}
		}
		if err == nil {
			out, err = a.app.Idempotent(
				r.Context(),
				p,
				x.IdempotencyKey,
				x,
				func() (any, error) {
					return a.app.CreateTopicsBatch(r.Context(), p, x.RoadmapID, x.Topics)
				},
			)
		}

	case "update_topic":
		var x struct {
			IdempotencyKey string     `json:"idempotency_key"`
			TopicID        uuid.UUID  `json:"topic_id"`
			Title          *string    `json:"title"`
			Description    *string    `json:"description"`
			ParentID       *uuid.UUID `json:"parent_id"`
			SortOrder      *int       `json:"sort_order"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.TopicID == uuid.Nil {
			err = domain.ValidationError{Field: "topic_id", Reason: "is required"}
		}
		if err == nil {
			patch := TopicPatch{
				Title:       x.Title,
				Description: x.Description,
				SortOrder:   x.SortOrder,
			}
			if bytesPresent(c.Arguments, "parent_id") {
				patch.ParentSet = true
				patch.ParentID = x.ParentID
			}
			out, err = a.app.Idempotent(
				r.Context(),
				p,
				x.IdempotencyKey,
				x,
				func() (any, error) {
					return a.app.UpdateTopic(r.Context(), p, x.TopicID, patch)
				},
			)
		}

	case "set_dependencies_batch":
		var x struct {
			IdempotencyKey string              `json:"idempotency_key"`
			RoadmapID      uuid.UUID           `json:"roadmap_id"`
			Dependencies   []domain.Dependency `json:"dependencies"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.RoadmapID == uuid.Nil {
			err = domain.ValidationError{Field: "roadmap_id", Reason: "is required"}
		}
		if err == nil {
			out, err = a.app.Idempotent(
				r.Context(),
				p,
				x.IdempotencyKey,
				x,
				func() (any, error) {
					return map[string]bool{"ok": true}, a.app.SetDependencies(
						r.Context(),
						p,
						x.RoadmapID,
						x.Dependencies,
					)
				},
			)
		}

	case "set_topic_priority":
		var x struct {
			IdempotencyKey string    `json:"idempotency_key"`
			TopicID        uuid.UUID `json:"topic_id"`
			Priority       string    `json:"priority"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.TopicID == uuid.Nil {
			err = domain.ValidationError{Field: "topic_id", Reason: "is required"}
		}
		if err == nil {
			out, err = a.app.Idempotent(
				r.Context(),
				p,
				x.IdempotencyKey,
				x,
				func() (any, error) {
					return map[string]bool{"ok": true}, a.app.SetTopicPriority(
						r.Context(),
						p,
						x.TopicID,
						x.Priority,
					)
				},
			)
		}

	case "add_checklist_items":
		var x struct {
			IdempotencyKey string    `json:"idempotency_key"`
			TopicID        uuid.UUID `json:"topic_id"`
			Items          []struct {
				Text string `json:"text"`
			} `json:"items"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.TopicID == uuid.Nil {
			err = domain.ValidationError{Field: "topic_id", Reason: "is required"}
		}
		if err == nil {
			texts := make([]string, len(x.Items))
			for i, item := range x.Items {
				texts[i] = item.Text
			}
			out, err = a.app.Idempotent(
				r.Context(),
				p,
				x.IdempotencyKey,
				x,
				func() (any, error) {
					return a.app.AppendChecklist(r.Context(), p, x.TopicID, texts)
				},
			)
		}

	default:
		rpcError(w, id, -32601, "unknown tool")
		return
	}

	if err != nil {
		a.logger.Warn(
			"mcp tool failed",
			"operation", "mcp_tool",
			"tool", c.Name,
			"error_class", apperrors.Class(err),
		)
		rpcError(w, id, -32602, apperrors.MCP(err))
		return
	}

	toolResult(w, id, out)
}

func decodeMCPArguments(raw json.RawMessage, value any) error {
	if len(raw) == 0 {
		return domain.ValidationError{Field: "arguments", Reason: "is required"}
	}

	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return domain.ValidationError{Field: "arguments", Reason: "must match the tool input schema"}
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.ValidationError{Field: "arguments", Reason: "must contain one object"}
	}

	return nil
}

func (a *API) roadmapDetail(ctx context.Context, p domain.Principal, id uuid.UUID) (domain.RoadmapDetail, error) {
	r, err := a.app.GetRoadmap(ctx, p, id)
	if err != nil {
		return domain.RoadmapDetail{}, err
	}

	topics, err := a.app.ListTopics(ctx, p, id)
	if err != nil {
		return domain.RoadmapDetail{}, err
	}

	deps, err := a.app.Dependencies(ctx, p, id)
	if err != nil {
		return domain.RoadmapDetail{}, err
	}

	return domain.RoadmapDetail{
		Roadmap:      r,
		Topics:       topics,
		Dependencies: deps,
		Progress:     service.CalculateProgress(topics),
	}, nil
}

func bytesPresent(raw json.RawMessage, key string) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(raw, &m) == nil && m[key] != nil
}
