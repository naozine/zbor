package asr

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// WhisperConfig holds configuration for Whisper model
type WhisperConfig struct {
	ModelDir   string
	Language   string // ja, en, zh, etc. or empty for auto-detect
	Task       string // transcribe or translate
	NumThreads int
	SampleRate int
}

// DefaultWhisperConfig returns default Whisper configuration for Japanese
func DefaultWhisperConfig(modelDir string) *WhisperConfig {
	return &WhisperConfig{
		ModelDir:   modelDir,
		Language:   "ja",
		Task:       "transcribe",
		NumThreads: 4,
		SampleRate: 16000,
	}
}

// WhisperRecognizer wraps Whisper model for speech recognition
type WhisperRecognizer struct {
	recognizer *sherpa.OfflineRecognizer
	config     *WhisperConfig
}

// NewWhisperRecognizer creates a new Whisper recognizer
func NewWhisperRecognizer(config *WhisperConfig) (*WhisperRecognizer, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	// Find encoder and decoder files
	encoderCandidates := []string{
		"encoder.int8.onnx",
		"encoder.onnx",
		"turbo-encoder.int8.onnx",
		"turbo-encoder.onnx",
	}
	decoderCandidates := []string{
		"decoder.int8.onnx",
		"decoder.onnx",
		"turbo-decoder.int8.onnx",
		"turbo-decoder.onnx",
	}

	encoderPath := findModelFile(config.ModelDir, encoderCandidates)
	decoderPath := findModelFile(config.ModelDir, decoderCandidates)
	tokensPath := config.ModelDir + "/tokens.txt"

	if encoderPath == "" {
		return nil, fmt.Errorf("encoder model not found in %s", config.ModelDir)
	}
	if decoderPath == "" {
		return nil, fmt.Errorf("decoder model not found in %s", config.ModelDir)
	}
	if _, err := os.Stat(tokensPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("tokens file not found: %s", tokensPath)
	}

	sherpaConfig := sherpa.OfflineRecognizerConfig{
		FeatConfig: sherpa.FeatureConfig{
			SampleRate: config.SampleRate,
			FeatureDim: 80,
		},
		ModelConfig: sherpa.OfflineModelConfig{
			Whisper: sherpa.OfflineWhisperModelConfig{
				Encoder:  encoderPath,
				Decoder:  decoderPath,
				Language: config.Language,
				Task:     config.Task,
			},
			Tokens:     tokensPath,
			NumThreads: config.NumThreads,
			Debug:      0,
		},
	}

	recognizer := sherpa.NewOfflineRecognizer(&sherpaConfig)
	if recognizer == nil {
		return nil, fmt.Errorf("failed to create Whisper recognizer")
	}

	return &WhisperRecognizer{
		recognizer: recognizer,
		config:     config,
	}, nil
}


// Close releases the recognizer resources
func (r *WhisperRecognizer) Close() {
	if r.recognizer != nil {
		sherpa.DeleteOfflineRecognizer(r.recognizer)
		r.recognizer = nil
	}
}

// TranscribeFile transcribes an audio file using Whisper
func (r *WhisperRecognizer) TranscribeFile(inputPath string, chunkSec int, onProgress ProgressCallback) (*Result, error) {
	if chunkSec <= 0 {
		chunkSec = 30 // Whisper supports up to 30 seconds natively
	}

	if onProgress != nil {
		onProgress(10, "converting")
	}

	// Get duration for progress calculation
	duration, _ := getAudioDuration(inputPath)

	// Convert audio to raw PCM using ffmpeg
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ar", fmt.Sprintf("%d", r.config.SampleRate),
		"-ac", "1",
		"-loglevel", "error",
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	reader := bufio.NewReader(stdout)

	chunkSamples := r.config.SampleRate * chunkSec
	chunkBytes := chunkSamples * 2

	var allTokens []Token
	var allText strings.Builder
	chunkNum := 0
	var processedSamples int64

	if onProgress != nil {
		onProgress(20, "transcribing")
	}

	for {
		buffer := make([]byte, chunkBytes)
		n, err := io.ReadFull(reader, buffer)
		if n == 0 {
			break
		}

		samples := bytesToFloat32SV(buffer[:n]) // Reuse from sensevoice.go
		processedSamples += int64(len(samples))
		chunkNum++

		startSec := float32((chunkNum - 1) * chunkSec)

		// Transcribe chunk
		tokens := r.transcribeChunk(samples, startSec)
		if len(tokens) > 0 {
			allTokens = append(allTokens, tokens...)
			for _, t := range tokens {
				allText.WriteString(t.Text)
			}
		}

		// Update progress
		if onProgress != nil && duration > 0 {
			progress := 20 + int(60*float64(processedSamples)/float64(r.config.SampleRate)/duration)
			if progress > 80 {
				progress = 80
			}
			onProgress(progress, fmt.Sprintf("chunk %d", chunkNum))
		}

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}

	cmd.Wait()

	if onProgress != nil {
		onProgress(90, "finalizing")
	}

	// Calculate total duration
	var totalDuration float32
	if len(allTokens) > 0 {
		lastToken := allTokens[len(allTokens)-1]
		totalDuration = lastToken.StartTime + lastToken.Duration
	}

	return &Result{
		Text:          allText.String(),
		Tokens:        allTokens,
		Segments:      tokensToSegments(allTokens),
		TotalDuration: totalDuration,
	}, nil
}

// transcribeChunk transcribes a single audio chunk
func (r *WhisperRecognizer) transcribeChunk(samples []float32, timeOffset float32) []Token {
	if len(samples) == 0 {
		return nil
	}

	stream := sherpa.NewOfflineStream(r.recognizer)
	defer sherpa.DeleteOfflineStream(stream)

	stream.AcceptWaveform(r.config.SampleRate, samples)
	r.recognizer.Decode(stream)

	result := stream.GetResult()
	if result == nil {
		return nil
	}

	return extractTokensWithOffset(result, timeOffset)
}
