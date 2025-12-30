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
		"large-v3-encoder.int8.onnx",
		"large-v3-encoder.onnx",
		"large-v2-encoder.int8.onnx",
		"large-v2-encoder.onnx",
		"turbo-encoder.int8.onnx",
		"turbo-encoder.onnx",
	}
	decoderCandidates := []string{
		"decoder.int8.onnx",
		"decoder.onnx",
		"large-v3-decoder.int8.onnx",
		"large-v3-decoder.onnx",
		"large-v2-decoder.int8.onnx",
		"large-v2-decoder.onnx",
		"turbo-decoder.int8.onnx",
		"turbo-decoder.onnx",
	}

	tokensCandidates := []string{
		"tokens.txt",
		"large-v3-tokens.txt",
		"large-v2-tokens.txt",
	}

	encoderPath := findModelFile(config.ModelDir, encoderCandidates)
	decoderPath := findModelFile(config.ModelDir, decoderCandidates)
	tokensPath := findModelFile(config.ModelDir, tokensCandidates)

	if encoderPath == "" {
		return nil, fmt.Errorf("encoder model not found in %s", config.ModelDir)
	}
	if decoderPath == "" {
		return nil, fmt.Errorf("decoder model not found in %s", config.ModelDir)
	}
	if tokensPath == "" {
		return nil, fmt.Errorf("tokens file not found in %s", config.ModelDir)
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

// TranscribePartial transcribes a specific time range of an audio file
// Since Whisper doesn't return timestamps, we distribute them uniformly
func (r *WhisperRecognizer) TranscribePartial(filePath string, opts PartialTranscribeOptions) (*Result, error) {
	if opts.ChunkSec <= 0 {
		opts.ChunkSec = 30 // Whisper supports up to 30 seconds natively
	}

	duration := opts.EndTime - opts.StartTime
	if duration <= 0 {
		return nil, fmt.Errorf("invalid time range: %.2f - %.2f", opts.StartTime, opts.EndTime)
	}

	// Build ffmpeg command to extract and process the time range
	args := []string{
		"-ss", fmt.Sprintf("%.3f", opts.StartTime),
		"-i", filePath,
		"-t", fmt.Sprintf("%.3f", duration),
	}

	// Add tempo filter if not 1.0
	if opts.Tempo > 0 && opts.Tempo != 1.0 {
		args = append(args, "-af", fmt.Sprintf("atempo=%.2f", opts.Tempo))
	}

	args = append(args,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ar", fmt.Sprintf("%d", r.config.SampleRate),
		"-ac", "1",
		"-loglevel", "error",
		"pipe:1",
	)

	cmd := exec.Command("ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Read all audio data
	reader := bufio.NewReader(stdout)
	var allSamples []float32

	chunkBytes := r.config.SampleRate * opts.ChunkSec * 2
	for {
		buffer := make([]byte, chunkBytes)
		n, err := io.ReadFull(reader, buffer)
		if n == 0 {
			break
		}
		samples := bytesToFloat32SV(buffer[:n])
		allSamples = append(allSamples, samples...)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}
	cmd.Wait()

	if len(allSamples) == 0 {
		return &Result{}, nil
	}

	// Transcribe all samples at once (Whisper handles up to 30s well)
	stream := sherpa.NewOfflineStream(r.recognizer)
	defer sherpa.DeleteOfflineStream(stream)

	stream.AcceptWaveform(r.config.SampleRate, allSamples)
	r.recognizer.Decode(stream)

	result := stream.GetResult()
	if result == nil || result.Text == "" {
		return &Result{}, nil
	}

	// Use Whisper's tokens (word/subword level) instead of character splitting
	text := strings.TrimSpace(result.Text)
	tokens := distributeTimestampsToWhisperTokens(result.Tokens, opts.StartTime, opts.EndTime)

	return &Result{
		Text:   text,
		Tokens: tokens,
	}, nil
}

// distributeTimestampsToWhisperTokens creates tokens with uniformly distributed timestamps
// using Whisper's word/subword tokens instead of character-level splitting
func distributeTimestampsToWhisperTokens(whisperTokens []string, startTime, endTime float64) []Token {
	// Filter out empty tokens
	var validTokens []string
	for _, t := range whisperTokens {
		if strings.TrimSpace(t) != "" {
			validTokens = append(validTokens, t)
		}
	}

	if len(validTokens) == 0 {
		return nil
	}

	duration := endTime - startTime
	tokenDuration := duration / float64(len(validTokens))

	tokens := make([]Token, len(validTokens))
	for i, t := range validTokens {
		tokens[i] = Token{
			Text:      t,
			StartTime: float32(startTime + float64(i)*tokenDuration),
			Duration:  float32(tokenDuration),
		}
	}
	return tokens
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
