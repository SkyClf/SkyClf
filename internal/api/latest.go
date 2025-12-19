package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/SkyClf/SkyClf/internal/infer"
	"github.com/SkyClf/SkyClf/internal/store"
)

type LatestHandler struct {
	st        *store.Store
	imagesDir string
	modelsDir string
	pred      infer.Predictor
}

func NewLatestHandler(st *store.Store, imagesDir string, modelsDir string, pred infer.Predictor) *LatestHandler {
	return &LatestHandler{
		st:        st,
		imagesDir: imagesDir,
		modelsDir: modelsDir,
		pred:      pred,
	}
}

func (h *LatestHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/latest", h.handleLatest)
	mux.HandleFunc("GET /api/clf", h.handleClf)
	mux.HandleFunc("POST /api/classify", h.handleClassifyUpload)
	mux.HandleFunc("GET /api/models/download", h.handleDownloadModel)
	mux.HandleFunc("GET /api/models/list", h.handleListModels)
}

// handleDownloadModel serves a model file for download, optionally by version
func (h *LatestHandler) handleDownloadModel(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	file := r.URL.Query().Get("file") // model.onnx or model.pt
	modelDir := filepath.Join(h.modelsDir, "skystate")
	var modelPath string

	// ensure deterministic ordering
	entries, err := os.ReadDir(modelDir)
	if err == nil && version == "" {
		var vers []string
		for _, e := range entries {
			if e.IsDir() && len(e.Name()) > 0 && e.Name()[0] == 'v' {
				vers = append(vers, e.Name())
			}
		}
		if len(vers) > 0 {
			sort.Strings(vers)
			version = vers[len(vers)-1]
		}
	}

	// Download specific version
	if version != "" {
		targetDir := filepath.Join(modelDir, version)
		candidates := []string{"model.onnx", "model.pt"}
		if file != "" {
			candidates = []string{file}
		}
		for _, fname := range candidates {
			tryPath := filepath.Join(targetDir, fname)
			if _, err := os.Stat(tryPath); err == nil {
				modelPath = tryPath
				break
			}
		}
	}
	if modelPath == "" {
		http.Error(w, "No model found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(modelPath)+"\"")
	http.ServeFile(w, r, modelPath)
}

// handleListModels lists all available model versions
func (h *LatestHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
	modelDir := filepath.Join(h.modelsDir, "skystate")
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		http.Error(w, "No models found", http.StatusNotFound)
		return
	}
	var models []map[string]any
	for _, e := range entries {
		if e.IsDir() && len(e.Name()) > 0 && e.Name()[0] == 'v' {
			version := e.Name()
			m := map[string]any{"version": version}
			// Optional metadata
			if meta, err := os.ReadFile(filepath.Join(modelDir, version, "meta.json")); err == nil {
				var metaMap map[string]any
				if json.Unmarshal(meta, &metaMap) == nil {
					if created, ok := metaMap["created_at"]; ok {
						m["created_at"] = created
					}
				}
			}
			for _, fname := range []string{"model.onnx", "model.pt"} {
				tryPath := filepath.Join(modelDir, version, fname)
				if _, err := os.Stat(tryPath); err == nil {
					m[fname] = "/api/models/download?version=" + version + "&file=" + fname
				}
			}
			models = append(models, m)
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i]["version"].(string) > models[j]["version"].(string) })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
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

// handleClassifyUpload runs inference against an uploaded image (test hook for the UI)
func (h *LatestHandler) handleClassifyUpload(w http.ResponseWriter, r *http.Request) {
	if h.pred == nil {
		http.Error(w, "no model loaded", http.StatusServiceUnavailable)
		return
	}

	if err := r.ParseMultipartForm(12 << 20); err != nil { // 12MB
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field 'file' required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "skyclf-upload-*")
	if err != nil {
		http.Error(w, "failed to buffer upload", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmp.Name())

	n, err := io.Copy(tmp, file)
	if err != nil {
		http.Error(w, "failed to read upload", http.StatusBadRequest)
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		http.Error(w, "failed to buffer upload", http.StatusInternalServerError)
		return
	}

	pred, err := h.pred.PredictImage(r.Context(), tmp.Name())
	if err != nil {
		http.Error(w, "prediction failed", http.StatusInternalServerError)
		return
	}
	if pred == nil {
		http.Error(w, "no prediction", http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"filename":   hdr.Filename,
		"size":       n,
		"prediction": pred,
	})
}
