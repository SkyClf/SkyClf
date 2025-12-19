package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SkyClf/SkyClf/internal/store"
)

type DatasetHandler struct {
	st *store.Store
}

func NewDatasetHandler(st *store.Store) *DatasetHandler {
	return &DatasetHandler{st: st}
}

func (h *DatasetHandler) RegisterRoutes(mux *http.ServeMux) {
	// NOTE: This conflicts with ImagesHandler which also registers GET /api/images
	// You should either disable ImagesHandler.listImages or change this route.
	mux.HandleFunc("GET /api/dataset/images", h.handleListImages)
	mux.HandleFunc("GET /api/dataset/stats", h.handleStats)
	mux.HandleFunc("POST /api/labels", h.handleSetLabel)
	mux.HandleFunc("POST /api/labels/reset", h.handleClearLabels)
}

func (h *DatasetHandler) handleListImages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	unlabeled := q.Get("unlabeled") == "1" || strings.EqualFold(q.Get("unlabeled"), "true")

	// limit=0 (default) means "no limit"
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}

	items, err := h.st.ListImages(limit, unlabeled)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"count": len(items),
		"items": items,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *DatasetHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.st.CountStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

type setLabelRequest struct {
	ImageID  string `json:"image_id"`
	Skystate string `json:"skystate"`
	Meteor   bool   `json:"meteor"`
}

func (h *DatasetHandler) handleSetLabel(w http.ResponseWriter, r *http.Request) {
	var req setLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	req.ImageID = strings.TrimSpace(req.ImageID)
	req.Skystate = strings.TrimSpace(req.Skystate)

	if req.ImageID == "" {
		http.Error(w, "image_id required", http.StatusBadRequest)
		return
	}
	if req.Skystate == "" {
		http.Error(w, "skystate required", http.StatusBadRequest)
		return
	}

	switch req.Skystate {
	case "clear", "light_clouds", "heavy_clouds", "precipitation", "unknown":
	default:
		http.Error(w, "invalid skystate value", http.StatusBadRequest)
		return
	}

	if err := h.st.SetLabel(req.ImageID, req.Skystate, req.Meteor, time.Now().UTC()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *DatasetHandler) handleClearLabels(w http.ResponseWriter, r *http.Request) {
	confirm := r.URL.Query().Get("confirm")
	if confirm != "yes" {
		http.Error(w, "confirmation required; pass ?confirm=yes", http.StatusBadRequest)
		return
	}
	if err := h.st.ClearLabels(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "all labels removed"})
}
