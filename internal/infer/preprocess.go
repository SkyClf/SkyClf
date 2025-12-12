package infer

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"os"

	xdraw "golang.org/x/image/draw"
)

const (
	imgSize = 224
)

// ImageNet normalization (matches your training)
var mean = [3]float32{0.485, 0.456, 0.406}
var std = [3]float32{0.229, 0.224, 0.225}

func LoadAndPreprocessNCHW(path string) ([]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	// Resize to 224x224
	dst := image.NewRGBA(image.Rect(0, 0, imgSize, imgSize))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)

	// NCHW: [1,3,224,224]
	out := make([]float32, 1*3*imgSize*imgSize)
	hw := imgSize * imgSize

	// channel-first
	for y := 0; y < imgSize; y++ {
		for x := 0; x < imgSize; x++ {
			c := dst.At(x, y)
			r8, g8, b8, _ := color.RGBAModel.Convert(c).RGBA()
			// r8 is 0..65535
			r := float32(r8) / 65535.0
			g := float32(g8) / 65535.0
			b := float32(b8) / 65535.0

			// normalize
			r = (r - mean[0]) / std[0]
			g = (g - mean[1]) / std[1]
			b = (b - mean[2]) / std[2]

			i := y*imgSize + x
			out[0*hw+i] = r
			out[1*hw+i] = g
			out[2*hw+i] = b
		}
	}

	if len(out) != 3*imgSize*imgSize {
		return nil, fmt.Errorf("unexpected tensor size: %d", len(out))
	}
	return out, nil
}
