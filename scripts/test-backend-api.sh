#!/usr/bin/env bash
# Runs the backend against an isolated, disposable PostgreSQL database and
# exercises every public REST route plus every advertised MCP tool.
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
project="knowledge-map-api-test-$$"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/knowledge-map-api-test.XXXXXX")"
compose_file="$work_dir/compose.test.yaml"
env_file="$work_dir/test.env"
cookie_jar="$work_dir/cookies.txt"
body_file="$work_dir/body"
header_file="$work_dir/headers"
mcp_token="api-test-token"

cleanup() {
	local status=$?
	docker compose -p "$project" --env-file "$env_file" -f "$root_dir/compose.yaml" -f "$compose_file" down --volumes --remove-orphans >/dev/null 2>&1 || true
	rm -rf "$work_dir"
	exit "$status"
}
trap cleanup EXIT

require() {
	command -v "$1" >/dev/null || { echo "missing required command: $1" >&2; exit 1; }
}

for command in docker jq; do require "$command"; done

cat >"$env_file" <<'EOF'
POSTGRES_DB=roadmap_api_test
POSTGRES_USER=roadmap_api_test
POSTGRES_PASSWORD=roadmap_api_test
ACCESS_KEY_PEPPER=api-test-pepper
MCP_STATIC_TOKEN=api-test-token
DOMAIN=invalid.test
EOF

cat >"$compose_file" <<EOF
services:
  backend:
    environment:
      ALLOW_BOOTSTRAP: "true"
      COOKIE_SECURE: "false"
      MCP_ENABLED: "true"
      MCP_STATIC_TOKEN: "api-test-token"
      MCP_SCOPES: "roadmaps:read,roadmaps:write"
      OWNER_LOGIN: "owner"
EOF

echo "Building and starting isolated backend…"
docker compose -p "$project" --env-file "$env_file" -f "$root_dir/compose.yaml" -f "$compose_file" up -d --build postgres backend >/dev/null

# Docker Desktop does not expose a container port to this sandbox's host
# namespace. Run curl in the application's internal network instead; files
# are mounted at their original absolute paths so cookie and response handling
# stays in this shell.
curl() {
	docker run --rm --network "${project}_edge" -v "${work_dir}:${work_dir}" curlimages/curl:8.12.1 "$@"
}

base_url="http://backend:8080"
backend_ready=false
for _ in {1..10}; do
	if curl --connect-timeout 1 --max-time 2 --fail --silent --show-error "$base_url/healthz" >/dev/null 2>&1; then
		backend_ready=true
		break
	fi
	sleep 1
done
if [[ "$backend_ready" != true ]]; then
	echo "backend did not become ready; its log follows:" >&2
	docker compose -p "$project" --env-file "$env_file" -f "$root_dir/compose.yaml" -f "$compose_file" logs backend >&2
	exit 1
fi

status=''
request() {
	local method=$1 path=$2 csrf=${3:-} payload=${4:-}
	local args=(-sS -o "$body_file" -D "$header_file" -w '%{http_code}' -b "$cookie_jar" -c "$cookie_jar" -X "$method")
	[[ -n "$csrf" ]] && args+=(-H "X-CSRF-Token: $csrf")
	[[ -n "$payload" ]] && args+=(-H 'Content-Type: application/json' --data "$payload")
	status="$(curl "${args[@]}" "$base_url$path")"
}

request_form() {
	local path=$1 field=$2 file=$3
	status="$(curl -sS -o "$body_file" -D "$header_file" -w '%{http_code}' -b "$cookie_jar" -c "$cookie_jar" -X POST -H "X-CSRF-Token: $csrf" -F "$field" -F "file=@$file;type=image/gif" "$base_url$path")"
}

expect() {
	local wanted=$1 label=$2
	if [[ "$status" != "$wanted" ]]; then
		echo "$label: expected HTTP $wanted, got $status" >&2
		cat "$body_file" >&2
		exit 1
	fi
	echo "✓ $label"
}

csrf_from_headers() { awk 'tolower($1) == "x-csrf-token:" { sub(/\r$/, "", $2); print $2 }' "$header_file"; }
json() { jq -er "$1" "$body_file"; }
uuid() { json "$1"; }

request GET /healthz
expect 200 health

request POST /api/auth/bootstrap '' '{"login":"owner","password":"a long safe test password"}'
expect 200 bootstrap
csrf="$(csrf_from_headers)"
[[ -n "$csrf" ]] || { echo "bootstrap did not issue CSRF token" >&2; exit 1; }

request GET /api/auth/me
expect 200 current-user

request POST /api/auth/key-login '' '{"key":"invalid"}'
expect 401 key-login-rejects-invalid-key

request POST /api/roadmaps "$csrf" '{"title":"API smoke roadmap","description":"isolated test"}'
expect 201 create-roadmap
roadmap_id="$(uuid '.id')"

request GET /api/roadmaps
expect 200 list-roadmaps
request GET "/api/roadmaps/$roadmap_id"
expect 200 get-roadmap
request PATCH "/api/roadmaps/$roadmap_id" "$csrf" '{"title":"Updated API smoke roadmap"}'
expect 200 update-roadmap

request POST "/api/roadmaps/$roadmap_id/topics" "$csrf" '{"title":"Leaf topic","description":"test","sort_order":1,"kind":"topic"}'
expect 201 create-topic
topic_id="$(uuid '.id')"
request POST "/api/roadmaps/$roadmap_id/topics" "$csrf" '{"title":"Second topic","description":"test","sort_order":2,"kind":"topic"}'
expect 201 create-second-topic
second_topic_id="$(uuid '.id')"
request GET "/api/roadmaps/$roadmap_id/topics"
expect 200 list-topics
request PATCH "/api/topics/$topic_id" "$csrf" '{"title":"Renamed topic","priority":"critical"}'
expect 200 update-topic
request PUT "/api/topics/$topic_id/status" "$csrf" '{"status":"in_progress"}'
expect 200 set-topic-status
request PUT "/api/topics/$topic_id/completion" "$csrf" '{"completed":true}'
expect 204 set-completion

request GET "/api/topics/$topic_id/note"
expect 200 get-note
request PUT "/api/topics/$topic_id/note" "$csrf" '{"content":"A private note"}'
expect 204 set-note
request GET "/api/topics/$topic_id/checklist"
expect 200 get-checklist
request PUT "/api/topics/$topic_id/checklist" "$csrf" '{"items":[{"text":"Read docs","completed":false,"sort_order":1}]}'
expect 200 set-checklist
request GET "/api/topics/$topic_id/document"
expect 200 get-document
request PUT "/api/topics/$topic_id/document" "$csrf" '{"content":"# Test document"}'
expect 200 set-document

request GET "/api/roadmaps/$roadmap_id/dependencies"
expect 200 get-dependencies
request PUT "/api/roadmaps/$roadmap_id/dependencies" "$csrf" "{\"dependencies\":[{\"predecessor_topic_id\":\"$topic_id\",\"successor_topic_id\":\"$second_topic_id\"}]}"
expect 204 set-dependencies
request GET "/api/roadmaps/$roadmap_id/progress"
expect 200 get-progress
request GET "/api/roadmaps/$roadmap_id/layout"
expect 200 get-layout
request PUT "/api/roadmaps/$roadmap_id/layout" "$csrf" "{\"items\":[{\"topic_id\":\"$topic_id\",\"x\":10,\"y\":20,\"collapsed\":false}]}"
expect 204 set-layout

request GET /api/analytics
expect 200 analytics
request GET '/api/learning/overview?from=2026-01-01&to=2026-12-31&timezone=UTC'
expect 200 learning-overview
request GET '/api/learning/activity?from=2026-01-01&to=2026-12-31&timezone=UTC'
expect 200 learning-activity
request GET '/api/learning/activity/2026-01-01?timezone=UTC'
expect 200 learning-activity-day
request GET /api/learning/achievements
expect 200 learning-achievements
request GET /api/learning/analysis/latest
expect 200 latest-learning-analysis-before-creation
request POST /api/learning/analysis "$csrf" '{}'
expect 200 create-learning-analysis

request GET /api/learning/sessions/active
expect 200 active-learning-session
request POST /api/learning/sessions/start "$csrf" "{\"elementId\":\"$topic_id\"}"
expect 201 start-learning-session
session_id="$(uuid '.id')"
request POST "/api/learning/sessions/$session_id/pause" "$csrf" '{}'
expect 200 pause-learning-session
request POST "/api/learning/sessions/$session_id/resume" "$csrf" '{}'
expect 200 resume-learning-session
request POST "/api/learning/sessions/$session_id/finish" "$csrf" '{"note":"done"}'
expect 200 finish-learning-session
request GET '/api/learning/sessions?from=2026-01-01&to=2026-12-31&timezone=UTC'
expect 200 list-learning-sessions

gif_file="$work_dir/image.gif"
# A valid 1×1 GIF; writing the bytes directly keeps the smoke test portable.
printf '\x47\x49\x46\x38\x39\x61\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff\x2c\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x01\x4c\x00\x3b' >"$gif_file"
status="$(curl -sS -o "$body_file" -D "$header_file" -w '%{http_code}' -b "$cookie_jar" -c "$cookie_jar" -X PUT -H "X-CSRF-Token: $csrf" -F "file=@$gif_file;type=image/gif" "$base_url/api/roadmaps/$roadmap_id/cover")"
expect 200 set-roadmap-cover
request GET "/api/roadmaps/$roadmap_id/cover"
expect 200 get-roadmap-cover
request_form /api/uploads/images "topic_id=$topic_id" "$gif_file"
expect 201 upload-image
image_id="$(uuid '.id')"
request GET "/api/uploads/images/$image_id"
expect 200 get-image
request DELETE "/api/roadmaps/$roadmap_id/cover" "$csrf"
expect 204 delete-roadmap-cover

mcp() {
	local payload=$1
	status="$(curl -sS -o "$body_file" -D "$header_file" -w '%{http_code}' -X POST -H "Authorization: Bearer $mcp_token" -H 'Content-Type: application/json' --data "$payload" "$base_url/mcp")"
}
mcp_get() {
	status="$(curl -sS -o "$body_file" -D "$header_file" -w '%{http_code}' -X GET -H "Authorization: Bearer $mcp_token" "$base_url/mcp")"
}
mcp_get
expect 405 mcp-get-method-not-allowed
mcp '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
expect 200 mcp-tools-list
mcp '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_roadmaps","arguments":{}}}'
expect 200 mcp-list-roadmaps
mcp "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"get_roadmap\",\"arguments\":{\"roadmap_id\":\"$roadmap_id\"}}}"
expect 200 mcp-get-roadmap
mcp '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_learning_statistics","arguments":{}}}'
expect 200 mcp-learning-statistics
mcp '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_learning_activity","arguments":{}}}'
expect 200 mcp-learning-activity
mcp '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"create_roadmap","arguments":{"idempotency_key":"mcp-create-roadmap","title":"MCP roadmap","description":"test"}}}'
expect 200 mcp-create-roadmap
mcp "{\"jsonrpc\":\"2.0\",\"id\":7,\"method\":\"tools/call\",\"params\":{\"name\":\"create_topics_batch\",\"arguments\":{\"idempotency_key\":\"mcp-create-topics\",\"roadmap_id\":\"$roadmap_id\",\"topics\":[{\"client_ref\":\"mcp-topic\",\"title\":\"MCP topic\",\"description\":\"test\",\"sort_order\":3}]}}}"
expect 200 mcp-create-topics-batch
mcp "{\"jsonrpc\":\"2.0\",\"id\":8,\"method\":\"tools/call\",\"params\":{\"name\":\"update_topic\",\"arguments\":{\"idempotency_key\":\"mcp-update-topic\",\"topic_id\":\"$topic_id\",\"title\":\"MCP updated topic\"}}}"
expect 200 mcp-update-topic
mcp "{\"jsonrpc\":\"2.0\",\"id\":9,\"method\":\"tools/call\",\"params\":{\"name\":\"set_topic_priority\",\"arguments\":{\"idempotency_key\":\"mcp-topic-priority\",\"topic_id\":\"$topic_id\",\"priority\":\"optional\"}}}"
expect 200 mcp-set-topic-priority
mcp "{\"jsonrpc\":\"2.0\",\"id\":10,\"method\":\"tools/call\",\"params\":{\"name\":\"set_dependencies_batch\",\"arguments\":{\"idempotency_key\":\"mcp-set-dependencies\",\"roadmap_id\":\"$roadmap_id\",\"dependencies\":[]}}}"
expect 200 mcp-set-dependencies
mcp "{\"jsonrpc\":\"2.0\",\"id\":11,\"method\":\"tools/call\",\"params\":{\"name\":\"add_checklist_items\",\"arguments\":{\"idempotency_key\":\"mcp-add-checklist\",\"topic_id\":\"$topic_id\",\"items\":[{\"text\":\"MCP task\"}]}}}"
expect 200 mcp-add-checklist-items

request DELETE "/api/topics/$second_topic_id" "$csrf"
expect 204 delete-topic
request DELETE "/api/roadmaps/$roadmap_id" "$csrf"
expect 204 delete-roadmap
request POST /api/auth/logout "$csrf"
expect 204 logout
request POST /api/auth/login '' '{"login":"owner","password":"a long safe test password"}'
expect 200 login

echo "Backend API smoke test passed."
