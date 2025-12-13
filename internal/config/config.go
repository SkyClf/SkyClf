package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Addr          string        // e.g. ":8080"
	AllSkyURL     string        // required for fetching
	PollInterval  time.Duration // e.g. 15s
	DataDir       string        // e.g. "./data"
	ModelsDir     string        // e.g. "./data/models"
	ImagesDir     string        // e.g. "./data/images"
	LabelsDBPath  string        // e.g. "./data/labels/labels.db"
	LogLevel      string        // "debug"|"info"|"warn"|"error"

	// Trainer settings
	TrainerContainer string // Container name for trainer, e.g. "skyclf-trainer"
}

func Load() (Config, error) {
	// Load .env file if present (ignore error if not found)
	_ = godotenv.Load()

	cfg := Config{
		Addr:         getenv("SKYCLF_ADDR", ":8080"),
		AllSkyURL:    strings.TrimSpace(os.Getenv("SKYCLF_ALLSKY_URL")),
		PollInterval: getenvDuration("SKYCLF_POLL_INTERVAL", 15*time.Second),
		DataDir:      getenv("SKYCLF_DATA_DIR", "./data"),
		LogLevel:     strings.ToLower(getenv("SKYCLF_LOG_LEVEL", "info")),
	}

	// Derived paths
	cfg.ModelsDir = getenv("SKYCLF_MODELS_DIR", cfg.DataDir+"/models")
	cfg.ImagesDir = getenv("SKYCLF_IMAGES_DIR", cfg.DataDir+"/images")
	cfg.LabelsDBPath = getenv("SKYCLF_LABELS_DB", cfg.DataDir+"/labels/labels.db")

	// Trainer settings
	cfg.TrainerContainer = getenv("SKYCLF_TRAINER_CONTAINER", "skyclf-trainer")

	// Validation
	var errs []string
	if cfg.AllSkyURL == "" {
		errs = append(errs, "SKYCLF_ALLSKY_URL is required (e.g. http://camera/latest.jpg)")
	}
	if cfg.PollInterval < 2*time.Second {
		errs = append(errs, "SKYCLF_POLL_INTERVAL too low; use >= 2s")
	}
	if cfg.LogLevel != "debug" && cfg.LogLevel != "info" && cfg.LogLevel != "warn" && cfg.LogLevel != "error" {
		errs = append(errs, "SKYCLF_LOG_LEVEL must be one of: debug, info, warn, error")
	}

	if len(errs) > 0 {
		return Config{}, errors.New(strings.Join(errs, "; "))
	}
	return cfg, nil
}

func getenv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func getenvDuration(key string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}

	// Accept "15s", "2m", etc.
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}

	// Also accept plain seconds: "15"
	if n, err := strconv.Atoi(raw); err == nil {
		return time.Duration(n) * time.Second
	}

	// Fall back to default, but keep it explicit via panic-like error pattern upstream:
	// We return def here and let Load() validation catch crazy values if needed.
	fmt.Fprintf(os.Stderr, "WARN: invalid %s=%q, using default %s\n", key, raw, def)
	return def
}
