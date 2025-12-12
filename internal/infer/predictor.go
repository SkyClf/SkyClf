package infer

import "context"

type Prediction struct {
	SkyState    string             `json:"skystate"`
	Confidence  float32            `json:"confidence"`
	Probs       map[string]float32 `json:"probs,omitempty"`
	ModelTask   string             `json:"task"`
	ModelVer    string             `json:"model_version"`
	ModelPath   string             `json:"model_path"`
}

type Predictor interface {
	PredictImage(ctx context.Context, imagePath string) (*Prediction, error)
	Close() error
}
