package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
	"roadmap/backend/internal/service"
)

func (a *API) listTopics(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	v, e := a.app.ListTopics(r.Context(), principal(r), id)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": v})
}

func (a *API) createTopic(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v domain.TopicInput
	if !decode(w, r, &v) {
		return
	}
	out, e := a.app.CreateTopic(r.Context(), principal(r), id, v)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 201, out)
}

func (a *API) updateTopic(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var raw map[string]json.RawMessage
	if !decode(w, r, &raw) {
		return
	}

	var in service.TopicPatch
	var e error
	if b, ok := raw["parent_id"]; ok {
		in.ParentSet = true
		if string(b) != "null" {
			var x uuid.UUID
			e = json.Unmarshal(b, &x)
			in.ParentID = &x
		}
	}

	if b, ok := raw["title"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.Title = &x
	}
	if b, ok := raw["description"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.Description = &x
	}

	if b, ok := raw["sort_order"]; ok {
		var x int
		e = json.Unmarshal(b, &x)
		in.SortOrder = &x
	}
	if b, ok := raw["kind"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.Kind = &x
	}
	if b, ok := raw["task_status"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.TaskStatus = &x
	}
	if b, ok := raw["status"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.Status = &x
	}
	if b, ok := raw["priority"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.Priority = &x
	}

	if e != nil {
		writeError(w, 400, "invalid request", nil)
		return
	}
	out, e := a.app.UpdateTopic(r.Context(), principal(r), id, in)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, out)
}

func (a *API) setTopicStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &v) || v.Status == "" {
		writeError(w, 400, "status is required", nil)
		return
	}
	if err := a.app.SetTopicStatus(r.Context(), principal(r), id, v.Status); err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": v.Status})
}

func (a *API) deleteTopic(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if e := a.app.DeleteTopic(r.Context(), principal(r), id); e != nil {
		respondErr(w, e)
		return
	}
	w.WriteHeader(204)
}

func (a *API) setCompletion(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v struct {
		Completed *bool `json:"completed"`
	}
	if !decode(w, r, &v) || v.Completed == nil {
		if v.Completed == nil {
			writeError(w, 400, "completed is required", nil)
		}
		return
	}
	if e := a.app.SetCompletion(r.Context(), principal(r), id, *v.Completed); e != nil {
		respondErr(w, e)
		return
	}
	w.WriteHeader(204)
}
