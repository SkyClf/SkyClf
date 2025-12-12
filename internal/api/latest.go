package api

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/SkyClf/SkyClf/internal/infer"
	"github.com/SkyClf/SkyClf/internal/store"
)

type LatestHandler struct {
	st        *store.Store
	imagesDir string
	pred      infer.Predictor
}

func NewLatestHandler(st *store.Store, imagesDir string, pred infer.Predictor) *LatestHandler {
	return &LatestHandler{
		st:        st,
		imagesDir: imagesDir,
		pred:      pred,
	}
}

func (h *LatestHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/latest", h.handleLatest)
	mux.HandleFunc("GET /api/clf", h.handleClf)
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
		"prediction": h.getPrediction(r, latest.Path),
	})
}

// getPrediction runs inference if a model is loaded, otherwise returns nil
func (h *LatestHandler) getPrediction(r *http.Request, imagePath string) *infer.Prediction {
	if h.pred == nil {
		return nil
	}
	pred, _ := h.pred.PredictImage(r.Context(), imagePath) // ignore error for stability
	return pred
}

// handleClf returns only the prediction for the latest image - simple and easy to use
// GET /api/clf -> {"skystate": "heavy_clouds", "confidence": 0.998, "probs": {...}}
func (h *LatestHandler) handleClf(w http.ResponseWriter, r *http.Request) {
	latest, err := h.st.GetLatest()
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if latest == nil {
		http.Error(w, "no image", http.StatusNotFound)
		return
	}

	if h.pred == nil {
		http.Error(w, "no model loaded", http.StatusServiceUnavailable)
		return
	}

	pred, err := h.pred.PredictImage(r.Context(), latest.Path)
	if err != nil {
		http.Error(w, "prediction failed", http.StatusInternalServerError)
		return
	}
	if pred == nil {
		http.Error(w, "no prediction", http.StatusServiceUnavailable)
		return
	}

	// Simple response: just skystate, confidence, probs
	writeJSON(w, http.StatusOK, map[string]any{
		"skystate":   pred.SkyState,
		"confidence": pred.Confidence,
		"probs":      pred.Probs,
	})
}