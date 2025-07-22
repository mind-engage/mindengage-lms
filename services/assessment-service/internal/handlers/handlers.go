// === internal/handlers/handlers.go ===
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/mind-engage/mindengage-lms/services/assessment-service/pkg/qti"
)

// Handler ties HTTP routes to the QTI engine
type Handler struct {
	engine qti.Engine
	store  map[uuid.UUID]qti.Item
}

// NewHandler creates a new Handler
func NewHandler(engine qti.Engine) *Handler {
	return &Handler{
		engine: engine,
		store:  make(map[uuid.UUID]qti.Item),
	}
}

// LaunchLTI initiates an LTI launch (stub)
func (h *Handler) LaunchLTI(w http.ResponseWriter, r *http.Request) {
	// TODO: validate LTI request, extract launch context
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"LTI launch received"}`))
}

// ListItems returns all QTI items in memory
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	items := make([]qti.Item, 0, len(h.store))
	for _, it := range h.store {
		items = append(items, it)
	}
	json.NewEncoder(w).Encode(items)
}

// CreateItem adds a new QTI item
func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	var req qti.CreateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, err)
		return
	}
	item := qti.Item{
		ID:        uuid.New(),
		Question:  req.Question,
		Options:   req.Options,
		AnswerKey: req.AnswerKey,
	}
	h.store[item.ID] = item
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(item)
}

// GetItem fetches a QTI item by ID
func (h *Handler) GetItem(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "itemID")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, err)
		return
	}
	item, ok := h.store[id]
	if !ok {
		h.respondError(w, http.StatusNotFound, err)
		return
	}
	json.NewEncoder(w).Encode(item)
}

// SubmitItem grades a submitted answer
func (h *Handler) SubmitItem(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "itemID")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, err)
		return
	}
	item, ok := h.store[id]
	if !ok {
		h.respondError(w, http.StatusNotFound, err)
		return
	}

	var req qti.SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, err)
		return
	}
	// Delegate to engine for scoring
	score := h.engine.Score(item, req.SelectedOption)
	resp := qti.SubmitResponse{Score: score}
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) respondError(w http.ResponseWriter, code int, err error) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
