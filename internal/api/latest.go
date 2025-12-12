package api

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/SkyClf/SkyClf/internal/store"
)

type LatestHandler struct {
	st *store.Store
}

func NewLatestHandler(st *store.Store) *LatestHandler {
	return &LatestHandler{st: st}
}

func (h *LatestHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/latest", h.handleLatest)
}

func (h *LatestHandler) handleLatest(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()

	latest, err := h.st.GetLatest()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if latest == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "no_image",
			"timestamp": now.Format(time.RFC3339),
			"image":     nil,
			"label":     nil,
		})
		return
	}

	filename := filepath.Base(latest.Path)

	// label: if not labeled yet => unknown
	skystate := "unknown"
	var meteor any = nil
	var labeledAt any = nil
	if latest.SkyState != nil {
		skystate = *latest.SkyState
	}
	if latest.Meteor != nil {
		meteor = *latest.Meteor
	}
	if latest.LabeledAt != nil {
		labeledAt = latest.LabeledAt.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"timestamp": now.Format(time.RFC3339),
		"image": map[string]any{
			"id":         latest.ID,
			"sha256":     latest.SHA256,
			"fetched_at": latest.FetchedAt.Format(time.RFC3339),
			"url":        "/images/" + filename, // specific file
			"latest_url": "/latest.jpg",         // always points to newest file
		},
		"label": map[string]any{
			"skystate":   skystate,
			"meteor":     meteor,
			"labeled_at": labeledAt,
		},
	})
}
