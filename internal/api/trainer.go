package api

import (
	"encoding/json"
	"net/http"

	"github.com/SkyClf/SkyClf/internal/trainer"
)

// TrainerHandler handles training API endpoints
type TrainerHandler struct {
	trainer *trainer.Trainer
}

// NewTrainerHandler creates a new trainer API handler
func NewTrainerHandler(t *trainer.Trainer) *TrainerHandler {
	return &TrainerHandler{trainer: t}
}

// RegisterRoutes registers the trainer API routes
func (h *TrainerHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/train/status", h.getStatus)
	mux.HandleFunc("POST /api/train/start", h.startTraining)
	mux.HandleFunc("POST /api/train/stop", h.stopTraining)
}

// GET /api/train/status - Get current training status
func (h *TrainerHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	status := h.trainer.Status(r.Context())
	writeJSON(w, http.StatusOK, status)
}

// POST /api/train/start - Start a training job
// Request body: { "epochs": 10, "batch_size": 16, "lr": "0.001", ... }
func (h *TrainerHandler) startTraining(w http.ResponseWriter, r *http.Request) {
	cfg := trainer.DefaultTrainConfig()

	// Parse optional overrides from request body
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid request body: " + err.Error(),
			})
			return
		}
	}

	// Validate
	if cfg.Epochs < 1 || cfg.Epochs > 1000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "epochs must be between 1 and 1000",
		})
		return
	}
	if cfg.BatchSize < 1 || cfg.BatchSize > 256 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "batch_size must be between 1 and 256",
		})
		return
	}

	if err := h.trainer.Start(r.Context(), cfg); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"message": "training started",
	})
}

// POST /api/train/stop - Stop the running training job
func (h *TrainerHandler) stopTraining(w http.ResponseWriter, r *http.Request) {
	if err := h.trainer.Stop(r.Context()); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "training stopped",
	})
}
