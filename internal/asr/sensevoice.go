package asr

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// SenseVoiceConfig holds configuration for SenseVoice model
type SenseVoiceConfig struct {
	ModelDir   string
	Language   string // zh, en, ja, ko, yue, auto
	UseInt8    bool
	NumThreads int
	SampleRate int
}

// DefaultSenseVoiceConfig returns default SenseVoice configuration
func DefaultSenseVoiceConfig(modelDir string) *SenseVoiceConfig {
	return &SenseVoiceConfig{
		ModelDir:   modelDir,
		Language:   "ja",
		UseInt8:    true,
		NumThreads: 4,
		SampleRate: 16000,
	}
}

// SenseVoiceRecognizer wraps SenseVoice model for speech recognition
type SenseVoiceRecognizer struct {
	recognizer *sherpa.OfflineRecognizer
	config     *SenseVoiceConfig
}

// NewSenseVoiceRecognizer creates a new SenseVoice recognizer
func NewSenseVoiceRecognizer(config *SenseVoiceConfig) (*SenseVoiceRecognizer, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	modelFile := "model.onnx"
	if config.UseInt8 {
		modelFile = "model.int8.onnx"
	}

	modelPath := config.ModelDir + "/" + modelFile
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("model file not found: %s", modelPath)
	}

	tokensPath := config.ModelDir + "/tokens.txt"
	if _, err := os.Stat(tokensPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("tokens file not found: %s", tokensPath)
	}

	sherpaConfig := sherpa.OfflineRecognizerConfig{
		FeatConfig: sherpa.FeatureConfig{
			SampleRate: config.SampleRate,
			FeatureDim: 80,
		},
		ModelConfig: sherpa.OfflineModelConfig{
			SenseVoice: sherpa.OfflineSenseVoiceModelConfig{
				Model:                       modelPath,
				Language:                    config.Language,
				UseInverseTextNormalization: 1,
			},
			Tokens:     tokensPath,
			NumThreads: config.NumThreads,
			Debug:      0,
		},
	}

	recognizer := sherpa.NewOfflineRecognizer(&sherpaConfig)
	if recognizer == nil {
		return nil, fmt.Errorf("failed to create SenseVoice recognizer")
	}

	return &SenseVoiceRecognizer{
		recognizer: recognizer,
		config:     config,
	}, nil
}

// Close releases the recognizer resources
func (r *SenseVoiceRecognizer) Close() {
	if r.recognizer != nil {
		sherpa.DeleteOfflineRecognizer(r.recognizer)
		r.recognizer = nil
	}
}

// TranscribeFile transcribes an audio file using SenseVoice
func (r *SenseVoiceRecognizer) TranscribeFile(inputPath string, chunkSec int, onProgress ProgressCallback) (*Result, error) {
	if chunkSec <= 0 {
		chunkSec = 20
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

		samples := bytesToFloat32SV(buffer[:n])
		processedSamples += int64(len(samples))
		chunkNum++

		startSec := float32((chunkNum - 1) * chunkSec)

		// Transcribe chunk and get tokens with timestamps
		tokens := r.transcribeBytes(samples, startSec)
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

// transcribeBytes transcribes raw audio samples and returns tokens with timestamps
func (r *SenseVoiceRecognizer) transcribeBytes(samples []float32, timeOffset float32) []Token {
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

	// Extract tokens with timestamps (same as ReazonSpeech)
	return extractTokensWithOffset(result, timeOffset)
}

// extractTokensWithOffset extracts tokens from result and adds time offset
func extractTokensWithOffset(result *sherpa.OfflineRecognizerResult, timeOffset float32) []Token {
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
			startTime = result.Timestamps[i] + timeOffset
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

// bytesToFloat32SV converts bytes to float32 samples
func bytesToFloat32SV(data []byte) []float32 {
	samples := make([]float32, len(data)/2)
	for i := 0; i < len(samples); i++ {
		sample := int16(binary.LittleEndian.Uint16(data[i*2:]))
		samples[i] = float32(sample) / 32768.0
	}
	return samples
}

// getAudioDuration gets audio duration using ffprobe
func getAudioDuration(path string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	var duration float64
	fmt.Sscanf(string(output), "%f", &duration)
	return duration, nil
}
