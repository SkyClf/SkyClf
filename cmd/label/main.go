package main

import (
	"flag"
	"log"
	"time"

	"github.com/SkyClf/SkyClf/internal/config"
	"github.com/SkyClf/SkyClf/internal/store"
)

func main() {
	state := flag.String("state", "heavy_clouds", "skystate to set for all images")
	meteor := flag.Bool("meteor", false, "set meteor flag for all images")
	limit := flag.Int("limit", 0, "max images to process (0 = all)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	st, err := store.Open(cfg.LabelsDBPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	images, err := st.ListImages(*limit, false)
	if err != nil {
		log.Fatalf("list images: %v", err)
	}

	now := time.Now()
	for _, img := range images {
		if err := st.SetLabel(img.ID, *state, *meteor, now); err != nil {
			log.Printf("set label %s for %s: %v", *state, img.ID, err)
		}
	}

	log.Printf("labeled %d images as %s", len(images), *state)
}
