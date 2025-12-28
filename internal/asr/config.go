package asr

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the configuration for the ASR recognizer
type Config struct {
	ModelPath   string // Base directory for the model
	EncoderPath string // Path to encoder.onnx or encoder.int8.onnx
	DecoderPath string // Path to decoder.onnx or decoder.int8.onnx
	JoinerPath  string // Path to joiner.onnx or joiner.int8.onnx
	TokensPath  string // Path to tokens.txt
	NumThreads  int    // Number of threads for inference
	SampleRate  int    // Audio sample rate (typically 16000)
}

// DefaultReazonSpeechConfig returns the default configuration for ReazonSpeech model
// Assumes the model is downloaded to the models directory
func DefaultReazonSpeechConfig() *Config {
	modelDir := "models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01"
	return &Config{
		ModelPath:   modelDir,
		EncoderPath: filepath.Join(modelDir, "encoder-epoch-99-avg-1.int8.onnx"),
		DecoderPath: filepath.Join(modelDir, "decoder-epoch-99-avg-1.onnx"),
		JoinerPath:  filepath.Join(modelDir, "joiner-epoch-99-avg-1.int8.onnx"),
		TokensPath:  filepath.Join(modelDir, "tokens.txt"),
		NumThreads:  2,
		SampleRate:  16000,
	}
}

// NewConfig creates a new configuration from a model directory
// It automatically detects the model files in the directory
func NewConfig(modelDir string) (*Config, error) {
	config := &Config{
		ModelPath:  modelDir,
		NumThreads: 2,
		SampleRate: 16000,
	}

	// Try to find model files (prefer int8 quantized versions)
	encoderPath := findModelFile(modelDir, []string{
		"encoder-epoch-99-avg-1.int8.onnx",
		"encoder.int8.onnx",
		"encoder-epoch-99-avg-1.onnx",
		"encoder.onnx",
	})
	if encoderPath == "" {
		return nil, fmt.Errorf("encoder model not found in %s", modelDir)
	}
	config.EncoderPath = encoderPath

	decoderPath := findModelFile(modelDir, []string{
		"decoder-epoch-99-avg-1.onnx",
		"decoder.onnx",
	})
	if decoderPath == "" {
		return nil, fmt.Errorf("decoder model not found in %s", modelDir)
	}
	config.DecoderPath = decoderPath

	joinerPath := findModelFile(modelDir, []string{
		"joiner-epoch-99-avg-1.int8.onnx",
		"joiner.int8.onnx",
		"joiner-epoch-99-avg-1.onnx",
		"joiner.onnx",
	})
	if joinerPath == "" {
		return nil, fmt.Errorf("joiner model not found in %s", modelDir)
	}
	config.JoinerPath = joinerPath

	tokensPath := findModelFile(modelDir, []string{"tokens.txt"})
	if tokensPath == "" {
		return nil, fmt.Errorf("tokens.txt not found in %s", modelDir)
	}
	config.TokensPath = tokensPath

	return config, nil
}

// Validate checks if all required model files exist
func (c *Config) Validate() error {
	files := map[string]string{
		"encoder": c.EncoderPath,
		"decoder": c.DecoderPath,
		"joiner":  c.JoinerPath,
		"tokens":  c.TokensPath,
	}

	for name, path := range files {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("%s file not found: %s", name, path)
		}
	}

	return nil
}

// findModelFile searches for a model file in the given directory
// Returns the first matching file path or empty string if not found
func findModelFile(dir string, candidates []string) string {
	for _, candidate := range candidates {
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
