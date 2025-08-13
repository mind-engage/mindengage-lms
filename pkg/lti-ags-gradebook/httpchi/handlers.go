package httpchi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/mind-engage/ags-gradebook/gradebook"
)

type API struct{ Syncer *gradebook.Syncer }

func (a *API) Routes(r chi.Router) {
	r.Post("/lti/gradebook/link", a.postLink)
	r.Post("/lti/gradebook/resync", a.postResync)
}

type linkReq struct {
	AttemptID                                       string `json:"attempt_id,omitempty"`
	ExamID                                          string `json:"exam_id"`
	Issuer, DeploymentID, ContextID, ResourceLinkID string
}

func (a *API) postLink(w http.ResponseWriter, r *http.Request) {
	var req linkReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.AttemptID != "" {
		if err := a.Syncer.SyncAttempt(req.AttemptID); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}
	at := gradebook.Attempt{
		ID: "ensure-only", ExamID: req.ExamID,
		PlatformIssuer: req.Issuer, DeploymentID: req.DeploymentID, ContextID: req.ContextID, ResourceLinkID: req.ResourceLinkID,
	}
	if _, err := a.Syncer.EnsureLineItem(at); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type resyncReq struct {
	AttemptID string `json:"attempt_id"`
}

func (a *API) postResync(w http.ResponseWriter, r *http.Request) {
	var req resyncReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.AttemptID == "" {
		http.Error(w, "attempt_id required", 400)
		return
	}
	if err := a.Syncer.SyncAttempt(req.AttemptID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
