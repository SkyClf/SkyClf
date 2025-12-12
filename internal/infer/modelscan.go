package infer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ModelInfo struct {
	Version    string
	Dir        string
	OnnxPath   string
	Classes    map[string]int
	ClassNames []string // index->name
}

func FindLatestSkyStateModel(modelsDir string) (*ModelInfo, error) {
	root := filepath.Join(modelsDir, "skystate")
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read models root: %w", err)
	}

	var vers []string
	for _, e := range ents {
		if e.IsDir() && strings.HasPrefix(e.Name(), "v") {
			vers = append(vers, e.Name())
		}
	}
	if len(vers) == 0 {
		return nil, nil
	}
	sort.Strings(vers)
	v := vers[len(vers)-1]

	dir := filepath.Join(root, v)
	onnxPath := filepath.Join(dir, "model.onnx")
	classesPath := filepath.Join(dir, "classes.json")

	if _, err := os.Stat(onnxPath); err != nil {
		return nil, nil // treat as "no model"
	}
	b, err := os.ReadFile(classesPath)
	if err != nil {
		return nil, fmt.Errorf("read classes.json: %w", err)
	}
	var classes map[string]int
	if err := json.Unmarshal(b, &classes); err != nil {
		return nil, fmt.Errorf("parse classes.json: %w", err)
	}

	// build index->name
	max := -1
	for _, id := range classes {
		if id > max {
			max = id
		}
	}
	if max < 0 {
		return nil, errors.New("classes.json empty")
	}
	names := make([]string, max+1)
	for name, id := range classes {
		if id >= 0 && id < len(names) {
			names[id] = name
		}
	}
	for i, n := range names {
		if n == "" {
			return nil, fmt.Errorf("classes.json missing name for id %d", i)
		}
	}

	return &ModelInfo{
		Version:    v,
		Dir:        dir,
		OnnxPath:   onnxPath,
		Classes:    classes,
		ClassNames: names,
	}, nil
}
