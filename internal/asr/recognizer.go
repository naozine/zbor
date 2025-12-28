package asr

import (
	"fmt"
	"os"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// Recognizer handles speech recognition using Sherpa-ONNX
type Recognizer struct {
	config     *Config
	recognizer *sherpa.OfflineRecognizer
}

// NewRecognizer creates a new ASR recognizer with the given configuration
func NewRecognizer(config *Config) (*Recognizer, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create sherpa-onnx configuration
	sherpaConfig := sherpa.OfflineRecognizerConfig{
		FeatConfig: sherpa.FeatureConfig{
			SampleRate: config.SampleRate,
			FeatureDim: 80,
		},
		ModelConfig: sherpa.OfflineModelConfig{
			Transducer: sherpa.OfflineTransducerModelConfig{
				Encoder: config.EncoderPath,
				Decoder: config.DecoderPath,
				Joiner:  config.JoinerPath,
			},
			Tokens:    config.TokensPath,
			NumThreads: config.NumThreads,
			Debug:     0,
		},
	}

	// Create recognizer
	recognizer := sherpa.NewOfflineRecognizer(&sherpaConfig)
	if recognizer == nil {
		return nil, fmt.Errorf("failed to create offline recognizer")
	}

	return &Recognizer{
		config:     config,
		recognizer: recognizer,
	}, nil
}

// TranscribeFile transcribes audio from a WAV file
func (r *Recognizer) TranscribeFile(audioPath string) (*Result, error) {
	startTime := time.Now()

	// Read audio file
	samples, err := r.readWavFile(audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio file: %w", err)
	}

	// Create stream
	stream := sherpa.NewOfflineStream(r.recognizer)
	defer sherpa.DeleteOfflineStream(stream)

	// Accept waveform
	stream.AcceptWaveform(r.config.SampleRate, samples)

	// Decode
	r.recognizer.Decode(stream)

	// Get result
	result := stream.GetResult()
	text := result.Text

	duration := time.Since(startTime).Seconds()

	return &Result{
		Text:     text,
		Segments: []Segment{}, // TODO: Extract segments if available
		Duration: duration,
	}, nil
}

// TranscribeBytes transcribes audio from raw audio samples
func (r *Recognizer) TranscribeBytes(samples []float32, sampleRate int) (*Result, error) {
	startTime := time.Now()

	// Create stream
	stream := sherpa.NewOfflineStream(r.recognizer)
	defer sherpa.DeleteOfflineStream(stream)

	// Accept waveform
	stream.AcceptWaveform(sampleRate, samples)

	// Decode
	r.recognizer.Decode(stream)

	// Get result
	result := stream.GetResult()
	text := result.Text

	duration := time.Since(startTime).Seconds()

	return &Result{
		Text:     text,
		Segments: []Segment{}, // TODO: Extract segments if available
		Duration: duration,
	}, nil
}

// Close releases resources used by the recognizer
func (r *Recognizer) Close() error {
	if r.recognizer != nil {
		sherpa.DeleteOfflineRecognizer(r.recognizer)
		r.recognizer = nil
	}
	return nil
}

// readWavFile reads a WAV file and returns the audio samples
func (r *Recognizer) readWavFile(path string) ([]float32, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	// Use sherpa-onnx's built-in WAV reader
	samples := sherpa.ReadWave(path)
	if samples == nil || len(samples.Samples) == 0 {
		return nil, fmt.Errorf("failed to read WAV file or file is empty")
	}

	return samples.Samples, nil
}
