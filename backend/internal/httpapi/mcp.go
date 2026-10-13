package httpapi

import (
	"encoding/json"
	"net/http"
)

func rpcError(w http.ResponseWriter, id any, code int, msg string) {
	writeJSON(w, 200, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
	})
}

func (a *API) mcp(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(405)
		return
	}

	var q struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      any             `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if !decode(w, r, &q) {
		return
	}
	if q.JSONRPC != "2.0" {
		rpcError(w, q.ID, -32600, "invalid request")
		return
	}

	switch q.Method {
	case "initialize":
		version := "2025-03-26" // retained for clients using the initial MVP handshake.
		var init struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(q.Params, &init) == nil && init.ProtocolVersion >= "2026-07-28" {
			version = "2026-07-28"
		}
		w.Header().Set("MCP-Protocol-Version", version)
		w.Header().Set("Mcp-Method", q.Method)
		w.Header().Set("Mcp-Name", "knowledge-roadmap")
		writeJSON(w, 200, map[string]any{
			"jsonrpc": "2.0",
			"id":      q.ID,
			"result": map[string]any{
				"protocolVersion": version,
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]string{
					"name":    "knowledge-roadmap",
					"version": "0.1.0",
				},
			},
		})

	case "tools/list":
		writeJSON(w, 200, map[string]any{
			"jsonrpc": "2.0",
			"id":      q.ID,
			"result": map[string]any{
				"tools": mcpTools(),
			},
		})

	case "tools/call":
		a.callTool(w, r, q.ID, q.Params)

	default:
		rpcError(w, q.ID, -32601, "method not found")
	}
}

func toolResult(w http.ResponseWriter, id any, result any) {
	writeJSON(w, 200, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": mustJSON(result),
				},
			},
			"isError":    false,
			"resultType": "complete",
		},
	})
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
