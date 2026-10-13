package httpapi

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
	"roadmap/backend/internal/repository"
	"roadmap/backend/internal/security"
	"roadmap/backend/internal/service"
)

func TestMeRestoresBoundCSRFAndMutationRequiresIt(t *testing.T) {
	store := newFakeStore()
	app := service.New(store)
	api := New(app, Config{SecureCookies: false}, nil)
	h := api.Router()

	me := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	me.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	me.AddCookie(&http.Cookie{Name: "roadmap_csrf", Value: store.csrfToken})
	meRR := httptest.NewRecorder()
	h.ServeHTTP(meRR, me)
	if meRR.Code != http.StatusOK {
		t.Fatalf("me status = %d: %s", meRR.Code, meRR.Body.String())
	}
	if got := meRR.Header().Get("X-CSRF-Token"); got != store.csrfToken {
		t.Fatalf("csrf header = %q", got)
	}

	topic := uuid.New()
	missing := httptest.NewRequest(http.MethodPut, "/api/topics/"+topic.String()+"/completion", bytes.NewBufferString(`{"completed":true}`))
	missing.Header.Set("Content-Type", "application/json")
	missing.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	missingRR := httptest.NewRecorder()
	h.ServeHTTP(missingRR, missing)
	if missingRR.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d", missingRR.Code)
	}

	ok := httptest.NewRequest(http.MethodPut, "/api/topics/"+topic.String()+"/completion", bytes.NewBufferString(`{"completed":true}`))
	ok.Header.Set("Content-Type", "application/json")
	ok.Header.Set("X-CSRF-Token", store.csrfToken)
	ok.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	okRR := httptest.NewRecorder()
	h.ServeHTTP(okRR, ok)
	if okRR.Code != http.StatusNoContent || !store.completion {
		t.Fatalf("completion status = %d, stored=%t", okRR.Code, store.completion)
	}
}

func TestMCPBearerAndScopes(t *testing.T) {
	store := newFakeStore()
	api := New(service.New(store), Config{MCPEnabled: true, MCPToken: "trusted", MCPOwnerLogin: store.user.Login, MCPScopes: []string{domain.ScopeRead}}, nil)
	h := api.Router()
	request := func(token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr
	}
	if rr := request("", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`); rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous mcp=%d", rr.Code)
	}
	if rr := request("trusted", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`); rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte("inputSchema")) {
		t.Fatalf("tools response: %d %s", rr.Code, rr.Body.String())
	}
	if rr := request("trusted", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_roadmap","arguments":{"idempotency_key":"operation-1","title":"Blocked","description":""}}}`); rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte("access denied")) {
		t.Fatalf("write scope response: %d %s", rr.Code, rr.Body.String())
	}
}

func TestLearningRangeAcceptsDateOnlyInTimezone(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/learning/activity?from=2026-10-11&to=2026-10-12&timezone=Europe/Moscow", nil)
	from, to, loc, err := learningRange(r)
	if err != nil {
		t.Fatal(err)
	}
	if from.Location() != loc || from.Format("2006-01-02") != "2026-10-11" || to.Format("2006-01-02") != "2026-10-12" {
		t.Fatalf("range=%s..%s loc=%s", from, to, loc)
	}
}

func TestMCPToolContractSchemasAndCompleteResult(t *testing.T) {
	for _, tool := range mcpTools() {
		schema, ok := tool["inputSchema"].(map[string]any)
		if !ok || schema["type"] != "object" || schema["additionalProperties"] != false || schema["properties"] == nil {
			t.Fatalf("invalid schema for %v: %#v", tool["name"], schema)
		}
	}
	store := newFakeStore()
	api := New(service.New(store), Config{MCPEnabled: true, MCPToken: "trusted", MCPOwnerLogin: store.user.Login, MCPScopes: []string{domain.ScopeRead}}, nil)
	r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_roadmaps","arguments":{}}}`))
	r.Header.Set("Authorization", "Bearer trusted")
	rr := httptest.NewRecorder()
	api.Router().ServeHTTP(rr, r)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte(`"resultType":"complete"`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`"isError":false`)) {
		t.Fatalf("result contract: %d %s", rr.Code, rr.Body.String())
	}
	bad := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_roadmaps","arguments":{"unknown":true}}}`))
	bad.Header.Set("Authorization", "Bearer trusted")
	badRR := httptest.NewRecorder()
	api.Router().ServeHTTP(badRR, bad)
	if !bytes.Contains(badRR.Body.Bytes(), []byte("invalid input")) {
		t.Fatalf("unknown argument accepted: %s", badRR.Body.String())
	}
}

func TestMCPSetTopicPriority(t *testing.T) {
	store := newFakeStore()
	topicID := uuid.New()
	api := New(service.New(store), Config{MCPEnabled: true, MCPToken: "trusted", MCPOwnerLogin: store.user.Login, MCPScopes: []string{domain.ScopeRead, domain.ScopeWrite}}, nil)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"set_topic_priority","arguments":{"idempotency_key":"priority-set-1","topic_id":"` + topicID.String() + `","priority":"critical"}}}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	r.Header.Set("Authorization", "Bearer trusted")
	rr := httptest.NewRecorder()
	api.Router().ServeHTTP(rr, r)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte(`\"ok\":true`)) {
		t.Fatalf("priority update failed: %d %s", rr.Code, rr.Body.String())
	}
	invalidBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"set_topic_priority","arguments":{"idempotency_key":"priority-set-2","topic_id":"` + topicID.String() + `","priority":"urgent"}}}`
	invalid := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(invalidBody))
	invalid.Header.Set("Authorization", "Bearer trusted")
	invalidRR := httptest.NewRecorder()
	api.Router().ServeHTTP(invalidRR, invalid)
	if !bytes.Contains(invalidRR.Body.Bytes(), []byte("invalid input")) {
		t.Fatalf("invalid priority accepted: %s", invalidRR.Body.String())
	}
}

func TestMCPAddChecklistItems(t *testing.T) {
	store := newFakeStore()
	api := New(service.New(store), Config{MCPEnabled: true, MCPToken: "trusted", MCPOwnerLogin: store.user.Login, MCPScopes: []string{domain.ScopeRead, domain.ScopeWrite}}, nil)
	topicID := uuid.New()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"add_checklist_items","arguments":{"idempotency_key":"checklist-add-1","topic_id":"` + topicID.String() + `","items":[{"text":"Прочитать главу"},{"text":"Решить задачи"}]}}}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	r.Header.Set("Authorization", "Bearer trusted")
	rr := httptest.NewRecorder()
	api.Router().ServeHTTP(rr, r)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte(`Прочитать главу`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`\"completed\":false`)) {
		t.Fatalf("checklist add failed: %d %s", rr.Code, rr.Body.String())
	}
}

func TestBootstrapConflictAndGroupCompletionAreRejected(t *testing.T) {
	store := newFakeStore()
	store.bootstrapErr = domain.ErrConflict
	api := New(service.New(store), Config{}, nil)
	bootstrap := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewBufferString(`{"login":"owner","password":"a long safe test password"}`))
	bootstrap.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	api.Router().ServeHTTP(rr, bootstrap)
	if rr.Code != http.StatusConflict {
		t.Fatalf("repeat bootstrap = %d: %s", rr.Code, rr.Body.String())
	}
	store.completionErr = domain.ErrConflict
	topic := uuid.New()
	complete := httptest.NewRequest(http.MethodPut, "/api/topics/"+topic.String()+"/completion", bytes.NewBufferString(`{"completed":true}`))
	complete.Header.Set("Content-Type", "application/json")
	complete.Header.Set("X-CSRF-Token", store.csrfToken)
	complete.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	completeRR := httptest.NewRecorder()
	api.Router().ServeHTTP(completeRR, complete)
	if completeRR.Code != http.StatusConflict {
		t.Fatalf("group completion = %d: %s", completeRR.Code, completeRR.Body.String())
	}
}

func TestLoginLimiterUsesTrustedForwardedIPAndLogin(t *testing.T) {
	store := newFakeStore()
	api := New(service.New(store), Config{}, nil)
	request := func(login string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"login":"`+login+`","password":"nope"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Forwarded-For", "2001:db8::1, 10.0.0.1")
		rr := httptest.NewRecorder()
		api.Router().ServeHTTP(rr, r)
		return rr
	}
	for i := 0; i < 5; i++ {
		if rr := request("owner"); rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d", i, rr.Code)
		}
	}
	if rr := request("owner"); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("limited request = %d", rr.Code)
	}
	if rr := request("other"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("different login shared limit: %d", rr.Code)
	}
}

func TestListRoadmapsIncludesLeafProgress(t *testing.T) {
	store := newFakeStore()
	roadmap := domain.Roadmap{ID: uuid.New(), OwnerID: store.user.ID}
	leaf := uuid.New()
	group := uuid.New()
	store.roadmaps = []domain.Roadmap{roadmap}
	store.topics = []domain.Topic{{ID: group, RoadmapID: roadmap.ID}, {ID: leaf, RoadmapID: roadmap.ID, ParentID: &group, Completed: true}}
	items, err := service.New(store).ListRoadmaps(context.Background(), domain.Principal{UserID: store.user.ID, Scopes: map[string]bool{domain.ScopeRead: true}})
	if err != nil || len(items) != 1 || items[0].Progress == nil || items[0].Progress.Total != 1 || items[0].Progress.Completed != 1 {
		t.Fatalf("progress response: %#v, %v", items, err)
	}
}

func TestSetLayoutsPersistsLongChainWithoutReordering(t *testing.T) {
	store := newFakeStore()
	roadmap := uuid.New()
	items := make([]domain.Layout, 1400)
	for i := range items {
		items[i] = domain.Layout{TopicID: uuid.New(), X: float64(i * 300), Y: float64(i%2) * 170, Collapsed: i%17 == 0}
	}
	p := domain.Principal{UserID: store.user.ID, Scopes: map[string]bool{domain.ScopeWrite: true}}
	if err := service.New(store).SetLayouts(context.Background(), p, roadmap, items); err != nil {
		t.Fatalf("long layout rejected: %v", err)
	}
	if len(store.layouts) != len(items) {
		t.Fatalf("stored layout items = %d, want %d", len(store.layouts), len(items))
	}
	for _, index := range []int{0, len(items) / 2, len(items) - 1} {
		if store.layouts[index] != items[index] {
			t.Fatalf("layout item %d changed: got %#v want %#v", index, store.layouts[index], items[index])
		}
	}
}

func TestTopicDocumentIsSeparateFromNoteAndRequiresCSRF(t *testing.T) {
	store := newFakeStore()
	store.document = domain.TopicDocument{Content: "# Existing", CreatedAt: timePtr(time.Now().UTC().Add(-time.Hour)), UpdatedAt: timePtr(time.Now().UTC().Add(-time.Minute))}
	store.documentSet = true
	api := New(service.New(store), Config{}, nil)
	topic := uuid.New()
	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/topics/"+topic.String()+"/document", nil)
	unauthenticatedRR := httptest.NewRecorder()
	api.Router().ServeHTTP(unauthenticatedRR, unauthenticated)
	if unauthenticatedRR.Code != http.StatusUnauthorized {
		t.Fatalf("document GET without session = %d", unauthenticatedRR.Code)
	}
	get := httptest.NewRequest(http.MethodGet, "/api/topics/"+topic.String()+"/document", nil)
	get.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	getRR := httptest.NewRecorder()
	api.Router().ServeHTTP(getRR, get)
	if getRR.Code != http.StatusOK || !bytes.Contains(getRR.Body.Bytes(), []byte("# Existing")) {
		t.Fatalf("document GET = %d: %s", getRR.Code, getRR.Body.String())
	}

	missingCSRF := httptest.NewRequest(http.MethodPut, "/api/topics/"+topic.String()+"/document", bytes.NewBufferString(`{"content":"# New"}`))
	missingCSRF.Header.Set("Content-Type", "application/json")
	missingCSRF.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	missingRR := httptest.NewRecorder()
	api.Router().ServeHTTP(missingRR, missingCSRF)
	if missingRR.Code != http.StatusForbidden {
		t.Fatalf("document PUT without csrf = %d", missingRR.Code)
	}

	put := httptest.NewRequest(http.MethodPut, "/api/topics/"+topic.String()+"/document", bytes.NewBufferString(`{"content":"# New"}`))
	put.Header.Set("Content-Type", "application/json")
	put.Header.Set("X-CSRF-Token", store.csrfToken)
	put.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	putRR := httptest.NewRecorder()
	api.Router().ServeHTTP(putRR, put)
	if putRR.Code != http.StatusOK || !bytes.Contains(putRR.Body.Bytes(), []byte("# New")) || !bytes.Contains(putRR.Body.Bytes(), []byte("created_at")) || !store.documentSet {
		t.Fatalf("document PUT = %d: %s", putRR.Code, putRR.Body.String())
	}
	if store.note.Content != "" {
		t.Fatalf("document write changed short note: %#v", store.note)
	}
	persisted := httptest.NewRequest(http.MethodGet, "/api/topics/"+topic.String()+"/document", nil)
	persisted.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	persistedRR := httptest.NewRecorder()
	api.Router().ServeHTTP(persistedRR, persisted)
	if persistedRR.Code != http.StatusOK || !bytes.Contains(persistedRR.Body.Bytes(), []byte("# New")) {
		t.Fatalf("document GET after PUT = %d: %s", persistedRR.Code, persistedRR.Body.String())
	}
	for _, content := range []string{strings.Repeat("x", 200001), strings.Repeat(" ", 200001)} {
		if _, err := service.New(store).SetDocument(context.Background(), domain.Principal{UserID: store.user.ID, Scopes: map[string]bool{domain.ScopeWrite: true}}, topic, content); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("oversized document error for %q = %v", content[:1], err)
		}
	}
	if _, err := service.New(store).GetDocument(context.Background(), domain.Principal{UserID: uuid.New(), Scopes: map[string]bool{domain.ScopeRead: true}}, topic); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign document read error = %v", err)
	}
}

func TestAnalyticsUsesLeafProgressAndOwnerScope(t *testing.T) {
	store := newFakeStore()
	first, second := domain.Roadmap{ID: uuid.New(), OwnerID: store.user.ID, Title: "First"}, domain.Roadmap{ID: uuid.New(), OwnerID: store.user.ID, Title: "Second"}
	group := uuid.New()
	firstDone, firstOpen, secondDone := uuid.New(), uuid.New(), uuid.New()
	store.roadmaps = []domain.Roadmap{first, second}
	store.topics = []domain.Topic{
		{ID: group, RoadmapID: first.ID},
		{ID: firstDone, RoadmapID: first.ID, ParentID: &group, Completed: true},
		{ID: firstOpen, RoadmapID: first.ID, ParentID: &group},
		{ID: secondDone, RoadmapID: second.ID, Completed: true},
	}
	p := domain.Principal{UserID: store.user.ID, Scopes: map[string]bool{domain.ScopeRead: true}}
	analytics, err := service.New(store).Analytics(context.Background(), p)
	if err != nil {
		t.Fatalf("analytics: %v", err)
	}
	if analytics.RoadmapCount != 2 || analytics.LeafTopics.Total != 3 || analytics.LeafTopics.Completed != 2 || analytics.LeafTopics.Remaining != 1 || analytics.LeafTopics.Percent == nil || *analytics.LeafTopics.Percent != 2*100.0/3.0 {
		t.Fatalf("analytics totals: %#v", analytics)
	}
	if len(analytics.Roadmaps) != 2 || analytics.Roadmaps[0].ID != first.ID || analytics.Roadmaps[0].LeafTopics.Total != 2 || analytics.Roadmaps[1].LeafTopics.Total != 1 {
		t.Fatalf("analytics breakdown: %#v", analytics.Roadmaps)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/analytics", nil)
	request.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	rr := httptest.NewRecorder()
	New(service.New(store), Config{}, nil).Router().ServeHTTP(rr, request)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte(`"roadmap_count":2`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`"remaining":1`)) {
		t.Fatalf("analytics endpoint = %d: %s", rr.Code, rr.Body.String())
	}
	if _, err := service.New(store).Analytics(context.Background(), domain.Principal{UserID: uuid.New(), Scopes: map[string]bool{domain.ScopeRead: true}}); err == nil {
		t.Fatal("analytics unexpectedly ignored owner scope")
	}
	store.roadmaps = nil
	empty, err := service.New(store).Analytics(context.Background(), p)
	if err != nil || empty.RoadmapCount != 0 || empty.LeafTopics.Total != 0 || empty.LeafTopics.Remaining != 0 || empty.LeafTopics.Percent != nil {
		t.Fatalf("empty analytics = %#v, err=%v", empty, err)
	}
}

func TestRoadmapCoverUploadIsOwnerScopedAndCSRFProtected(t *testing.T) {
	store := newFakeStore()
	api := New(service.New(store), Config{}, nil)
	roadmap := uuid.New()
	var body bytes.Buffer
	multipartWriter := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="cover.png"`)
	header.Set("Content-Type", "image/png")
	part, err := multipartWriter.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("\x89PNG\r\n\x1a\ncover"))
	if err := multipartWriter.Close(); err != nil {
		t.Fatal(err)
	}
	missingCSRF := httptest.NewRequest(http.MethodPut, "/api/roadmaps/"+roadmap.String()+"/cover", bytes.NewReader(body.Bytes()))
	missingCSRF.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	missingCSRF.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	missingRR := httptest.NewRecorder()
	api.Router().ServeHTTP(missingRR, missingCSRF)
	if missingRR.Code != http.StatusForbidden {
		t.Fatalf("cover upload without csrf = %d", missingRR.Code)
	}

	var validBody bytes.Buffer
	validWriter := multipart.NewWriter(&validBody)
	validPart, err := validWriter.CreateFormFile("file", "cover.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = validPart.Write([]byte("\x89PNG\r\n\x1a\ncover"))
	if err := validWriter.Close(); err != nil {
		t.Fatal(err)
	}
	put := httptest.NewRequest(http.MethodPut, "/api/roadmaps/"+roadmap.String()+"/cover", bytes.NewReader(validBody.Bytes()))
	put.Header.Set("Content-Type", validWriter.FormDataContentType())
	put.Header.Set("X-CSRF-Token", store.csrfToken)
	put.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	putRR := httptest.NewRecorder()
	api.Router().ServeHTTP(putRR, put)
	if putRR.Code != http.StatusOK || !store.coverSet || store.cover.MIMEType != "image/png" {
		t.Fatalf("cover upload = %d: %s", putRR.Code, putRR.Body.String())
	}
	get := httptest.NewRequest(http.MethodGet, "/api/roadmaps/"+roadmap.String()+"/cover", nil)
	get.AddCookie(&http.Cookie{Name: "roadmap_session", Value: store.sessionToken})
	getRR := httptest.NewRecorder()
	api.Router().ServeHTTP(getRR, get)
	if getRR.Code != http.StatusOK || getRR.Header().Get("Content-Type") != "image/png" || getRR.Body.String() != "\x89PNG\r\n\x1a\ncover" {
		t.Fatalf("cover read = %d %q", getRR.Code, getRR.Body.String())
	}
}

func TestMCPDisabledAndCookieSameSite(t *testing.T) {
	store := newFakeStore()
	api := New(service.New(store), Config{CookieSameSite: http.SameSiteStrictMode}, nil)
	mcp := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	mcpRR := httptest.NewRecorder()
	api.Router().ServeHTTP(mcpRR, mcp)
	if mcpRR.Code != http.StatusNotFound {
		t.Fatalf("disabled MCP=%d", mcpRR.Code)
	}
	loginRR := httptest.NewRecorder()
	api.loginResponse(loginRR, service.LoginResult{User: store.user, SessionToken: store.sessionToken, CSRFToken: store.csrfToken, ExpiresAt: time.Now().Add(time.Hour)})
	if !bytes.Contains([]byte(loginRR.Header().Get("Set-Cookie")), []byte("SameSite=Strict")) {
		t.Fatalf("cookie policy: %s", loginRR.Header().Get("Set-Cookie"))
	}
}

type fakeStore struct {
	user                    domain.User
	sessionToken, csrfToken string
	session                 domain.Session
	completion              bool
	completionErr           error
	bootstrapErr            error
	roadmaps                []domain.Roadmap
	topics                  []domain.Topic
	layouts                 []domain.Layout
	note                    domain.Note
	document                domain.TopicDocument
	documentSet             bool
	cover                   domain.RoadmapCover
	coverSet                bool
}

func newFakeStore() *fakeStore {
	u := domain.User{ID: uuid.New(), Login: "owner"}
	token, _ := security.RandomToken("s_")
	csrf, _ := security.RandomToken("c_")
	return &fakeStore{user: u, sessionToken: token, csrfToken: csrf, session: domain.Session{ID: uuid.New(), UserID: u.ID, TokenHash: security.TokenHash(token), CSRFHash: security.TokenHash(csrf), ExpiresAt: time.Now().Add(time.Hour)}}
}
func (f *fakeStore) CountUsers(context.Context) (int, error) { return 1, nil }
func (f *fakeStore) BootstrapUser(context.Context, string, string) (domain.User, error) {
	if f.bootstrapErr != nil {
		return domain.User{}, f.bootstrapErr
	}
	return f.user, nil
}
func (f *fakeStore) CreateUser(context.Context, string, string) (domain.User, error) {
	return f.user, nil
}
func (f *fakeStore) FindUserByLogin(_ context.Context, login string) (domain.User, string, error) {
	if login != f.user.Login {
		return domain.User{}, "", domain.ErrNotFound
	}
	return f.user, "", nil
}
func (f *fakeStore) FindUser(_ context.Context, id uuid.UUID) (domain.User, error) {
	if id != f.user.ID {
		return domain.User{}, domain.ErrNotFound
	}
	return f.user, nil
}
func (f *fakeStore) CreateSession(context.Context, domain.Session) error { return nil }
func (f *fakeStore) FindSession(_ context.Context, hash []byte, _ time.Time) (domain.Session, error) {
	if !bytes.Equal(hash, f.session.TokenHash) {
		return domain.Session{}, domain.ErrNotFound
	}
	return f.session, nil
}
func (f *fakeStore) DeleteSession(context.Context, []byte) error { return nil }
func (f *fakeStore) ListRoadmaps(_ context.Context, owner uuid.UUID) ([]domain.Roadmap, error) {
	if owner != f.user.ID {
		return nil, domain.ErrNotFound
	}
	return f.roadmaps, nil
}
func (f *fakeStore) GetRoadmap(context.Context, uuid.UUID, uuid.UUID) (domain.Roadmap, error) {
	return domain.Roadmap{}, domain.ErrNotFound
}
func (f *fakeStore) CreateRoadmap(context.Context, uuid.UUID, string, string) (domain.Roadmap, error) {
	return domain.Roadmap{}, nil
}
func (f *fakeStore) UpdateRoadmap(context.Context, uuid.UUID, uuid.UUID, *string, *string) (domain.Roadmap, error) {
	return domain.Roadmap{}, nil
}
func (f *fakeStore) DeleteRoadmap(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeStore) GetRoadmapCover(_ context.Context, owner, _ uuid.UUID) (domain.RoadmapCover, error) {
	if owner != f.user.ID || !f.coverSet {
		return domain.RoadmapCover{}, domain.ErrNotFound
	}
	return f.cover, nil
}
func (f *fakeStore) SetRoadmapCover(_ context.Context, owner, _ uuid.UUID, mime string, data []byte) (domain.RoadmapCover, error) {
	if owner != f.user.ID {
		return domain.RoadmapCover{}, domain.ErrNotFound
	}
	f.cover = domain.RoadmapCover{MIMEType: mime, Data: append([]byte(nil), data...), UpdatedAt: time.Now().UTC()}
	f.coverSet = true
	return f.cover, nil
}
func (f *fakeStore) DeleteRoadmapCover(_ context.Context, owner, _ uuid.UUID) error {
	if owner != f.user.ID || !f.coverSet {
		return domain.ErrNotFound
	}
	f.coverSet = false
	return nil
}
func (f *fakeStore) ListTopics(_ context.Context, owner, roadmap uuid.UUID) ([]domain.Topic, error) {
	if owner != f.user.ID {
		return nil, domain.ErrNotFound
	}
	out := make([]domain.Topic, 0, len(f.topics))
	for _, topic := range f.topics {
		if topic.RoadmapID == roadmap {
			out = append(out, topic)
		}
	}
	return out, nil
}
func (f *fakeStore) GetTopic(context.Context, uuid.UUID, uuid.UUID) (domain.Topic, error) {
	return domain.Topic{}, domain.ErrNotFound
}
func (f *fakeStore) CreateTopic(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, string, string, int) (domain.Topic, error) {
	return domain.Topic{}, nil
}
func (f *fakeStore) CreateTopicsBatch(context.Context, uuid.UUID, uuid.UUID, []repository.BatchTopic) ([]domain.Topic, error) {
	return nil, nil
}
func (f *fakeStore) UpdateTopic(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, string, string, int) (domain.Topic, error) {
	return domain.Topic{}, nil
}
func (f *fakeStore) DeleteTopic(context.Context, uuid.UUID, uuid.UUID) error            { return nil }
func (f *fakeStore) SetTopicStatus(context.Context, uuid.UUID, uuid.UUID, string) error { return nil }
func (f *fakeStore) SetTopicPriority(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}
func (f *fakeStore) SetCompletion(context.Context, uuid.UUID, uuid.UUID, bool) error {
	if f.completionErr != nil {
		return f.completionErr
	}
	f.completion = true
	return nil
}
func (f *fakeStore) GetNote(context.Context, uuid.UUID, uuid.UUID) (domain.Note, error) {
	return f.note, nil
}
func (f *fakeStore) SetNote(_ context.Context, _ uuid.UUID, _ uuid.UUID, content string) error {
	f.note.Content = content
	return nil
}
func (f *fakeStore) ListChecklist(context.Context, uuid.UUID, uuid.UUID) ([]domain.ChecklistItem, error) {
	return []domain.ChecklistItem{}, nil
}
func (f *fakeStore) SetChecklist(_ context.Context, _ uuid.UUID, topicID uuid.UUID, items []domain.ChecklistItemInput) ([]domain.ChecklistItem, error) {
	out := make([]domain.ChecklistItem, len(items))
	for index, item := range items {
		id := uuid.New()
		if item.ID != nil {
			id = *item.ID
		}
		out[index] = domain.ChecklistItem{ID: id, TopicID: topicID, Text: item.Text, Completed: item.Completed, SortOrder: index}
	}
	return out, nil
}
func (f *fakeStore) AppendChecklist(_ context.Context, _ uuid.UUID, topicID uuid.UUID, texts []string) ([]domain.ChecklistItem, error) {
	out := make([]domain.ChecklistItem, len(texts))
	for index, text := range texts {
		out[index] = domain.ChecklistItem{ID: uuid.New(), TopicID: topicID, Text: text, SortOrder: index}
	}
	return out, nil
}
func (f *fakeStore) GetTopicDocument(_ context.Context, owner, _ uuid.UUID) (domain.TopicDocument, error) {
	if owner != f.user.ID {
		return domain.TopicDocument{}, domain.ErrNotFound
	}
	if !f.documentSet {
		return domain.TopicDocument{}, nil
	}
	return f.document, nil
}
func (f *fakeStore) SetTopicDocument(_ context.Context, owner, _ uuid.UUID, content string) (domain.TopicDocument, error) {
	if owner != f.user.ID {
		return domain.TopicDocument{}, domain.ErrNotFound
	}
	now := time.Now().UTC()
	if f.document.CreatedAt == nil {
		f.document.CreatedAt = &now
	}
	f.document.UpdatedAt = &now
	f.document.Content = content
	f.documentSet = true
	return f.document, nil
}
func (f *fakeStore) ListDependencies(context.Context, uuid.UUID, uuid.UUID) ([]domain.Dependency, error) {
	return nil, nil
}
func (f *fakeStore) ReplaceDependencies(context.Context, uuid.UUID, uuid.UUID, []domain.Dependency) error {
	return nil
}
func (f *fakeStore) ListLayouts(context.Context, uuid.UUID, uuid.UUID) ([]domain.Layout, error) {
	return f.layouts, nil
}
func (f *fakeStore) ReplaceLayouts(_ context.Context, _ uuid.UUID, _ uuid.UUID, items []domain.Layout) error {
	f.layouts = append([]domain.Layout(nil), items...)
	return nil
}
func (f *fakeStore) Audit(context.Context, uuid.UUID, string, string, string, *uuid.UUID, map[string]any) error {
	return nil
}

func timePtr(value time.Time) *time.Time { return &value }
func (f *fakeStore) GetIdempotency(context.Context, uuid.UUID, string, []byte) ([]byte, bool, error) {
	return nil, false, nil
}
func (f *fakeStore) PutIdempotency(context.Context, uuid.UUID, string, []byte, []byte) error {
	return nil
}
