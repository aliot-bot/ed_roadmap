package clog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestJSONLoggerWritesStructuredSafeFields(t *testing.T) {
	var output bytes.Buffer
	logger := NewJSON(&output, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger.Info("server started", "operation", "startup", "address", ":8080")

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log record: %v", err)
	}
	if record["msg"] != "server started" || record["operation"] != "startup" {
		t.Fatalf("unexpected log record: %#v", record)
	}
}
