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
			Tokens:     config.TokensPath,
			NumThreads: config.NumThreads,
			Debug:      0,
		},
		DecodingMethod: config.DecodingMethod,
		MaxActivePaths: config.MaxActivePaths,
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
	if result == nil {
		// Return empty result if recognition failed
		return &Result{}, nil
	}

	// Extract tokens with timestamps
	tokens := extractTokens(result)

	// Calculate total audio duration from last token
	var totalDuration float32
	if len(tokens) > 0 {
		lastToken := tokens[len(tokens)-1]
		totalDuration = lastToken.StartTime + lastToken.Duration
	}

	processingTime := time.Since(startTime).Seconds()

	return &Result{
		Text:          result.Text,
		Tokens:        tokens,
		Segments:      tokensToSegments(tokens),
		TotalDuration: totalDuration,
		Duration:      processingTime,
	}, nil
}

// TranscribeBytes transcribes audio from raw audio samples
func (r *Recognizer) TranscribeBytes(samples []float32, sampleRate int) (*Result, error) {
	// Minimum sample count check: ONNX model crashes with "Invalid input shape" on very short audio
	// Require at least 0.1 seconds of audio (1600 samples at 16kHz)
	minSamples := sampleRate / 10 // 0.1 seconds
	if len(samples) < minSamples {
		// Return empty result for audio too short to process
		return &Result{}, nil
	}

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
	if result == nil {
		// Return empty result if recognition failed
		return &Result{}, nil
	}

	// Extract tokens with timestamps
	tokens := extractTokens(result)

	// Calculate total audio duration from last token
	var totalDuration float32
	if len(tokens) > 0 {
		lastToken := tokens[len(tokens)-1]
		totalDuration = lastToken.StartTime + lastToken.Duration
	}

	processingTime := time.Since(startTime).Seconds()

	return &Result{
		Text:          result.Text,
		Tokens:        tokens,
		Segments:      tokensToSegments(tokens),
		TotalDuration: totalDuration,
		Duration:      processingTime,
	}, nil
}

// extractTokens extracts Token slice from Sherpa-ONNX result
func extractTokens(result *sherpa.OfflineRecognizerResult) []Token {
	if result == nil || len(result.Tokens) == 0 {
		return nil
	}

	tokens := make([]Token, 0, len(result.Tokens))
	for i, text := range result.Tokens {
		// Skip empty tokens
		if text == "" {
			continue
		}

		var startTime float32
		var duration float32

		if i < len(result.Timestamps) {
			startTime = result.Timestamps[i]
		}
		if i < len(result.Durations) {
			duration = result.Durations[i]
		}

		tokens = append(tokens, Token{
			Text:      text,
			StartTime: startTime,
			Duration:  duration,
		})
	}

	return tokens
}

// tokensToSegments groups tokens into segments for SRT output
// Groups tokens with gaps > 0.5s into separate segments
func tokensToSegments(tokens []Token) []Segment {
	if len(tokens) == 0 {
		return nil
	}

	const gapThreshold = 0.5 // seconds

	var segments []Segment
	var currentText string
	var segmentStart float64
	var lastEnd float32

	for i, token := range tokens {
		tokenEnd := token.StartTime + token.Duration

		if i == 0 {
			segmentStart = float64(token.StartTime)
			currentText = token.Text
			lastEnd = tokenEnd
			continue
		}

		// Check if there's a significant gap
		gap := token.StartTime - lastEnd
		if gap > gapThreshold {
			// Save current segment
			segments = append(segments, Segment{
				Text:      currentText,
				StartTime: segmentStart,
				EndTime:   float64(lastEnd),
			})
			// Start new segment
			segmentStart = float64(token.StartTime)
			currentText = token.Text
		} else {
			currentText += token.Text
		}
		lastEnd = tokenEnd
	}

	// Add final segment
	if currentText != "" {
		segments = append(segments, Segment{
			Text:      currentText,
			StartTime: segmentStart,
			EndTime:   float64(lastEnd),
		})
	}

	return segments
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
