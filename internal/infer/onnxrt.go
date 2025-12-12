package infer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	ort "github.com/yalue/onnxruntime_go"
)

type ORTPredictor struct {
	mu sync.Mutex

	model   *ModelInfo
	session *ort.Session[float32]

	inTensor  *ort.Tensor[float32]
	outTensor *ort.Tensor[float32]
}

func NewORTPredictor(modelsDir string) (*ORTPredictor, error) {
	// Optional: allow user to point to a specific shared library path
	// e.g. SKYCLF_ORT_LIB=/usr/local/lib/onnxruntime.so
	if p := os.Getenv("SKYCLF_ORT_LIB"); p != "" {
		ort.SetSharedLibraryPath(p)
	}

	// Global ORT env (should be called once per process)
	if !ort.IsInitialized() {
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("onnxruntime init: %w", err)
		}
	}

	log.Printf("[infer] scanning models in %s", modelsDir)
	mi, err := FindLatestSkyStateModel(modelsDir)
	if err != nil {
		return nil, err
	}
	if mi == nil {
		log.Printf("[infer] no model found")
		return nil, nil // no model yet
	}
	log.Printf("[infer] found model: %s (version=%s, classes=%v)", mi.OnnxPath, mi.Version, mi.ClassNames)

	// Create fixed-shape tensors (batch=1)
	inShape := ort.NewShape(1, 3, 224, 224)
	outShape := ort.NewShape(1, int64(len(mi.ClassNames)))

	inData := make([]float32, inShape.FlattenedSize())
	inTensor, err := ort.NewTensor(inShape, inData)
	if err != nil {
		return nil, fmt.Errorf("create input tensor: %w", err)
	}

	outTensor, err := ort.NewEmptyTensor[float32](outShape)
	if err != nil {
		_ = inTensor.Destroy()
		return nil, fmt.Errorf("create output tensor: %w", err)
	}

	// Session: must provide names + tensors up front (per library design)
	// ONNX Runtime looks for external data files (model.onnx.data) in the current
	// working directory, so we need to temporarily change to the model's directory.
	modelDir := filepath.Dir(mi.OnnxPath)
	origDir, err := os.Getwd()
	if err != nil {
		_ = inTensor.Destroy()
		_ = outTensor.Destroy()
		return nil, fmt.Errorf("get working dir: %w", err)
	}
	if err := os.Chdir(modelDir); err != nil {
		_ = inTensor.Destroy()
		_ = outTensor.Destroy()
		return nil, fmt.Errorf("chdir to model dir: %w", err)
	}
	defer os.Chdir(origDir)

	sess, err := ort.NewSession[float32](
		filepath.Base(mi.OnnxPath), // use just the filename since we're in the model dir
		[]string{"input"},
		[]string{"logits"},
		[]*ort.Tensor[float32]{inTensor},
		[]*ort.Tensor[float32]{outTensor},
	)
	if err != nil {
		_ = inTensor.Destroy()
		_ = outTensor.Destroy()
		return nil, fmt.Errorf("create session: %w", err)
	}

	log.Printf("[infer] ONNX session loaded successfully")
	return &ORTPredictor{
		model:     mi,
		session:   sess,
		inTensor:  inTensor,
		outTensor: outTensor,
	}, nil
}

func (p *ORTPredictor) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.session != nil {
		_ = p.session.Destroy()
	}
	if p.inTensor != nil {
		_ = p.inTensor.Destroy()
	}
	if p.outTensor != nil {
		_ = p.outTensor.Destroy()
	}
	// Note: DestroyEnvironment() is global; you can call it on shutdown if you want.
	return nil
}

func (p *ORTPredictor) PredictImage(ctx context.Context, imagePath string) (*Prediction, error) {
	if p == nil || p.session == nil || p.model == nil {
		return nil, nil // no model loaded
	}

	start := time.Now()

	// single-thread safety: tensors are reused
	p.mu.Lock()
	defer p.mu.Unlock()

	x, err := LoadAndPreprocessNCHW(imagePath) // []float32 len=3*224*224
	if err != nil {
		log.Printf("[infer] preprocess error: %v", err)
		return nil, err
	}

	// Copy into the preallocated input tensor buffer
	copy(p.inTensor.GetData(), x)

	// Run inference
	if err := p.session.Run(); err != nil {
		return nil, fmt.Errorf("onnx run: %w", err)
	}

	logits := p.outTensor.GetData() // length = num_classes
	probs := softmax(logits)

	// argmax
	bestIdx := 0
	best := probs[0]
	for i := 1; i < len(probs); i++ {
		if probs[i] > best {
			best = probs[i]
			bestIdx = i
		}
	}

	// Build probs map name->prob
	probMap := make(map[string]float32, len(probs))
	for i, name := range p.model.ClassNames {
		probMap[name] = probs[i]
	}

	result := &Prediction{
		SkyState:   p.model.ClassNames[bestIdx],
		Confidence: best,
		Probs:      probMap,
		ModelTask:  "skystate",
		ModelVer:   p.model.Version,
		ModelPath:  filepath.ToSlash(p.model.OnnxPath),
	}

	log.Printf("[infer] prediction: %s (%.1f%%) took %v", result.SkyState, result.Confidence*100, time.Since(start))
	return result, nil
}

func softmax(logits []float32) []float32 {
	out := make([]float32, len(logits))
	if len(logits) == 0 {
		return out
	}

	// numerical stability: subtract max
	maxV := logits[0]
	for _, v := range logits[1:] {
		if v > maxV {
			maxV = v
		}
	}

	var sum float64
	for i, v := range logits {
		ev := math.Exp(float64(v - maxV))
		out[i] = float32(ev)
		sum += ev
	}
	if sum == 0 {
		return out
	}
	inv := float32(1.0 / sum)
	for i := range out {
		out[i] *= inv
	}
	return out
}

// Optional helper if you want /api/models later
func (p *ORTPredictor) ModelJSON() ([]byte, error) {
	if p == nil || p.model == nil {
		return json.Marshal(map[string]any{"active": nil})
	}
	return json.Marshal(map[string]any{
		"active": p.model.Version,
		"path":   p.model.OnnxPath,
	})
}
