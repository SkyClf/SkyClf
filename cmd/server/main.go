package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/SkyClf/SkyClf/internal/api"
	"github.com/SkyClf/SkyClf/internal/config"
	"github.com/SkyClf/SkyClf/internal/fetcher"
	"github.com/SkyClf/SkyClf/internal/infer"
	"github.com/SkyClf/SkyClf/internal/store"
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
	defer func() { if pred != nil { _ = pred.Close() } }()

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

		if err := st.UpsertImage(imageID, ev.Path, ev.SHA256Hex, ev.FetchedAt); err != nil {
			log.Printf("db: upsert image error: %v", err)
		}
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

	latestHandler := api.NewLatestHandler(st, cfg.ImagesDir, pred)
	latestHandler.RegisterRoutes(mux)

	// Start server
	server := &http.Server{Addr: cfg.Addr, Handler: mux}
	go func() {
		<-ctx.Done()
		log.Println("shutting down server...")
		_ = server.Close()
	}()

	log.Fatal(server.ListenAndServe())
}
