package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/SkyClf/SkyClf/internal/api"
	"github.com/SkyClf/SkyClf/internal/config"
	"github.com/SkyClf/SkyClf/internal/fetcher"
	"github.com/SkyClf/SkyClf/internal/infer"
	"github.com/SkyClf/SkyClf/internal/store"
	"github.com/SkyClf/SkyClf/internal/trainer"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	pred, err := infer.NewORTPredictor(cfg.ModelsDir)
	if err != nil {
		log.Fatalf("infer init: %v", err)
	}
	defer func() {
		if pred != nil {
			_ = pred.Close()
		}
	}()

	// Open label DB (also stores images metadata)
	st, err := store.Open(cfg.LabelsDBPath)
	if err != nil {
		log.Fatalf("db error: %v", err)
	}
	defer st.Close()

	n, _ := st.CountLabeled()
	log.Printf("SkyClf starting addr=%s poll=%s allsky=%s labeled=%d", cfg.Addr, cfg.PollInterval, cfg.AllSkyURL, n)

	// Create context that cancels on interrupt
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Start the image fetcher in background + upsert new images into DB
	fetch := fetcher.New(cfg.AllSkyURL, cfg.ImagesDir, cfg.PollInterval, func(ev fetcher.NewImageEvent) {
		// Use filename (without .jpg) as image_id; stable + human readable
		imageID := ev.Filename
		if len(imageID) > 4 && imageID[len(imageID)-4:] == ".jpg" {
			imageID = imageID[:len(imageID)-4]
		}

		if err := st.UpsertImage(imageID, ev.Path, ev.SHA256Hex, ev.FetchedAt, int64(ev.SizeBytes)); err != nil {
			log.Printf("db: upsert image error: %v", err)
		}
	})

	// Enable auto-cleanup: delete oldest unlabeled images when count exceeds 30,000
	fetch.SetAutoCleanup(st, 30000, func(result store.CleanupResult) {
		log.Printf("auto-cleanup completed: deleted %d images", result.DeletedCount)
	})

	go func() {
		if err := fetch.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("fetcher error: %v", err)
		}
	}()

	// Set up HTTP routes
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Images API
	imagesHandler := api.NewImagesHandler(cfg.ImagesDir)
	imagesHandler.RegisterRoutes(mux)

	// Serve latest image directly at /latest.jpg
	mux.HandleFunc("GET /latest.jpg", imagesHandler.ServeLatestImage)

	// (Optional) config debug endpoint
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ok\n"))
	})

	// Dataset API (images list + labels)
	datasetHandler := api.NewDatasetHandler(st)
	datasetHandler.RegisterRoutes(mux)

	latestHandler := api.NewLatestHandler(st, cfg.ImagesDir, cfg.ModelsDir, pred)
	latestHandler.RegisterRoutes(mux)

	// Trainer API (start/stop/status)
	tr, err := trainer.NewTrainer(cfg.TrainerContainer)
	if err != nil {
		log.Printf("trainer init warning (training disabled): %v", err)
	} else {
		defer tr.Close()

		// Auto-reload model when training completess
		tr.OnComplete = func() {
			log.Printf("trainer: reloading models after training completion")
			if pred != nil {
				if err := pred.Reload(cfg.ModelsDir, ""); err != nil {
					log.Printf("trainer: model reload error: %v", err)
				}
			}
		}

		trainerHandler := api.NewTrainerHandler(tr)
		trainerHandler.RegisterRoutes(mux)
		log.Printf("trainer ready: container=%s", cfg.TrainerContainer)
	}

	// Models API (reload endpoint)
	mux.HandleFunc("POST /api/models/reload", func(w http.ResponseWriter, r *http.Request) {
		if pred == nil {
			http.Error(w, `{"error":"predictor not initialized"}`, http.StatusInternalServerError)
			return
		}
		version := r.URL.Query().Get("version")
		if err := pred.Reload(cfg.ModelsDir, version); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","message":"models reloaded"}`))
	})

	mux.HandleFunc("GET /api/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if pred == nil {
			w.Write([]byte(`{"active":null}`))
			return
		}
		data, err := pred.ModelJSON()
		if err != nil {
			http.Error(w, `{"error":"json encode"}`, http.StatusInternalServerError)
			return
		}
		w.Write(data)
	})

	// Serve frontend from ui/dist (built Vue app)
	uiDir := "./ui/dist"
	if _, err := os.Stat(uiDir); err == nil {
		fs := http.FileServer(http.Dir(uiDir))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Serve index.html for SPA routes (non-API, non-file requests)
			path := r.URL.Path
			if path != "/" && !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/latest") {
				// Check if file exists
				if _, err := os.Stat(uiDir + path); os.IsNotExist(err) {
					// SPA fallback: serve index.html
					http.ServeFile(w, r, uiDir+"/index.html")
					return
				}
			}
			fs.ServeHTTP(w, r)
		})
		log.Printf("serving frontend from %s", uiDir)
	} else {
		log.Printf("frontend not found at %s (run 'npm run build' in ui/)", uiDir)
	}

	// Start server
	server := &http.Server{Addr: cfg.Addr, Handler: mux}
	go func() {
		<-ctx.Done()
		log.Println("shutting down server...")
		_ = server.Close()
	}()

	log.Fatal(server.ListenAndServe())
}
