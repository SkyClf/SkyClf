package api

import (
	"encoding/json"
	"net/http"
	"os"
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
	mux.HandleFunc("GET /api/dataset/days", h.handleListDays)
	mux.HandleFunc("POST /api/labels", h.handleSetLabel)
	mux.HandleFunc("POST /api/labels/reset", h.handleClearLabels)
	mux.HandleFunc("POST /api/images/cleanup", h.handleCleanupImages)
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

	var day string
	if raw := strings.TrimSpace(q.Get("date")); raw != "" {
		if _, err := time.Parse("2006-01-02", raw); err != nil {
			http.Error(w, "invalid date format; use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		day = raw
	}

	items, err := h.st.ListImages(limit, unlabeled, day)
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

func (h *DatasetHandler) handleListDays(w http.ResponseWriter, r *http.Request) {
	days, err := h.st.ListDays()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days": days,
	})
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

// handleCleanupImages handles cleanup of unlabeled images
// Query params:
//   - day: Delete all unlabeled images from this specific day (YYYY-MM-DD)
//   - max_unlabeled: Delete oldest unlabeled to keep count under this threshold (default: no auto-cleanup)
//
// Files are also deleted from disk.
func (h *DatasetHandler) handleCleanupImages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	day := q.Get("day")
	maxUnlabeledStr := q.Get("max_unlabeled")

	var result store.CleanupResult
	var err error

	if day != "" {
		// Delete all unlabeled from a specific day
		result, err = h.st.DeleteUnlabeledByDay(day)
	} else if maxUnlabeledStr != "" {
		// Delete oldest unlabeled to keep count under threshold
		maxUnlabeled, parseErr := strconv.Atoi(maxUnlabeledStr)
		if parseErr != nil || maxUnlabeled < 0 {
			http.Error(w, "invalid max_unlabeled value", http.StatusBadRequest)
			return
		}
		result, err = h.st.DeleteOldestUnlabeled(maxUnlabeled)
	} else {
		http.Error(w, "must specify 'day' or 'max_unlabeled' parameter", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Also delete files from disk
	deletedFromDisk := 0
	for _, path := range result.DeletedPaths {
		if removeErr := os.Remove(path); removeErr == nil {
			deletedFromDisk++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"deleted_count":    result.DeletedCount,
		"deleted_from_disk": deletedFromDisk,
		"freed_bytes":      result.FreedBytes,
	})
}
