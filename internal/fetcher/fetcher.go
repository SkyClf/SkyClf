package fetcher

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type OnNewImageFunc func(ev NewImageEvent)

type NewImageEvent struct {
	Filename  string
	Path      string
	SHA256Hex string
	FetchedAt time.Time
	SizeBytes int
}

// Fetcher periodically downloads images from an AllSky camera URL.
type Fetcher struct {
	url          string
	imagesDir    string
	pollInterval time.Duration
	client       *http.Client
	lastHash     [32]byte // Hash of last saved image to avoid duplicates
	onNewImage   OnNewImageFunc
}

// New creates a new Fetcher.
func New(url, imagesDir string, pollInterval time.Duration, onNewImage OnNewImageFunc) *Fetcher {
	return &Fetcher{
		url:          url,
		imagesDir:    imagesDir,
		pollInterval: pollInterval,
		onNewImage:   onNewImage,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Start begins the polling loop. It blocks until the context is canceled.
func (f *Fetcher) Start(ctx context.Context) error {
	// Ensure images directory exists
	if err := os.MkdirAll(f.imagesDir, 0755); err != nil {
		return fmt.Errorf("create images dir: %w", err)
	}

	// Fetch immediately on start
	if err := f.fetchAndSave(); err != nil {
		log.Printf("fetcher: initial fetch failed: %v", err)
	}

	ticker := time.NewTicker(f.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("fetcher: stopping")
			return ctx.Err()
		case <-ticker.C:
			if err := f.fetchAndSave(); err != nil {
				log.Printf("fetcher: %v", err)
			}
		}
	}
}

// fetchAndSave downloads the image and saves it to disk only if it changed.
func (f *Fetcher) fetchAndSave() error {
	resp, err := f.client.Get(f.url)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", f.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: status %d", f.url, resp.StatusCode)
	}

	// Read entire image into memory to compute hash
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// Check if image changed
	hash := sha256.Sum256(data)
	if hash == f.lastHash {
		// keep quiet-ish if you want, but leaving log is fine
		log.Printf("fetcher: image unchanged, skipping")
		return nil
	}
	f.lastHash = hash

	fetchedAt := time.Now().UTC()

	// Generate filename with timestamp
	ts := fetchedAt.Format("20060102_150405")
	filename := fmt.Sprintf("%s.jpg", ts)
	fpath := filepath.Join(f.imagesDir, filename)

	// Write file
	if err := os.WriteFile(fpath, data, 0644); err != nil {
		return fmt.Errorf("write file %s: %w", fpath, err)
	}

	log.Printf("fetcher: saved %s (%d bytes)", filename, len(data))

	if f.onNewImage != nil {
		f.onNewImage(NewImageEvent{
			Filename:  filename,
			Path:      fpath,
			SHA256Hex: fmt.Sprintf("%x", hash[:]),
			FetchedAt: fetchedAt,
			SizeBytes: len(data),
		})
	}

	return nil
}

// LatestImage returns the path to the most recent image, or empty string if none.
func (f *Fetcher) LatestImage() (string, error) {
	entries, err := os.ReadDir(f.imagesDir)
	if err != nil {
		return "", err
	}

	var latest string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".jpg" {
			// Files are named with timestamps, so lexicographic sort works
			if e.Name() > latest {
				latest = e.Name()
			}
		}
	}

	if latest == "" {
		return "", nil
	}
	return filepath.Join(f.imagesDir, latest), nil
}
