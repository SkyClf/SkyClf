package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ImagesHandler handles requests to list and serve images.
type ImagesHandler struct {
	imagesDir string
}

// NewImagesHandler creates a new ImagesHandler.
func NewImagesHandler(imagesDir string) *ImagesHandler {
	return &ImagesHandler{imagesDir: imagesDir}
}

// ImageInfo represents metadata about an image.
type ImageInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// RegisterRoutes registers the image routes on the given mux.
func (h *ImagesHandler) RegisterRoutes(mux *http.ServeMux) {
	// List all images
	mux.HandleFunc("GET /api/images", h.listImages)

	// Get latest image info
	mux.HandleFunc("GET /api/images/latest", h.latestImage)

	// Serve image files
	mux.Handle("GET /images/", http.StripPrefix("/images/", http.FileServer(http.Dir(h.imagesDir))))
}

// listImages returns a JSON list of all images.
func (h *ImagesHandler) listImages(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(h.imagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, []ImageInfo{})
			return
		}
		http.Error(w, "failed to read images directory", http.StatusInternalServerError)
		return
	}

	var images []ImageInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".jpg") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		images = append(images, ImageInfo{
			Name: e.Name(),
			URL:  "/images/" + e.Name(),
			Size: info.Size(),
		})
	}

	// Sort by name descending (newest first since names are timestamps)
	sort.Slice(images, func(i, j int) bool {
		return images[i].Name > images[j].Name
	})

	writeJSON(w, http.StatusOK, images)
}

// latestImage returns info about the most recent image.
func (h *ImagesHandler) latestImage(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(h.imagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "no images found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to read images directory", http.StatusInternalServerError)
		return
	}

	var latest string
	var latestSize int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".jpg") {
			continue
		}
		if e.Name() > latest {
			latest = e.Name()
			if info, err := e.Info(); err == nil {
				latestSize = info.Size()
			}
		}
	}

	if latest == "" {
		http.Error(w, "no images found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, ImageInfo{
		Name: latest,
		URL:  "/images/" + latest,
		Size: latestSize,
	})
}

// ServeLatestImage serves the actual latest image file (for direct embedding).
func (h *ImagesHandler) ServeLatestImage(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(h.imagesDir)
	if err != nil || len(entries) == 0 {
		http.Error(w, "no images found", http.StatusNotFound)
		return
	}

	var latest string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".jpg") {
			continue
		}
		if e.Name() > latest {
			latest = e.Name()
		}
	}

	if latest == "" {
		http.Error(w, "no images found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, filepath.Join(h.imagesDir, latest))
}
