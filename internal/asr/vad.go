package asr

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// VADConfig holds configuration for Voice Activity Detection
type VADConfig struct {
	ModelPath          string  // Path to silero_vad.onnx
	Threshold          float32 // Speech detection threshold (0-1, default 0.5)
	MinSpeechDuration  float32 // Minimum speech duration in seconds (default 0.25)
	MinSilenceDuration float32 // Minimum silence duration to split (default 0.5)
	MaxBlockDuration   float64 // Maximum block duration before splitting (default 5.0)
}

// DefaultVADConfig returns default VAD configuration
func DefaultVADConfig(modelPath string) *VADConfig {
	return &VADConfig{
		ModelPath:          modelPath,
		Threshold:          0.5,
		MinSpeechDuration:  0.25,
		MinSilenceDuration: 0.5,
		MaxBlockDuration:   5.0,
	}
}

// ProgressCallback is called to report transcription progress
type ProgressCallback func(progressPercent int, currentStep string)

// TranscribeWithVAD transcribes an audio/video file using VAD for efficient processing
// It uses ffmpeg to convert to raw PCM and VAD to detect speech segments
func (r *Recognizer) TranscribeWithVAD(inputPath string, vadConfig *VADConfig, onProgress ProgressCallback) (*Result, error) {
	// Get audio duration for progress calculation
	duration, err := GetAudioDuration(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get audio duration: %w", err)
	}

	// Check VAD model exists
	if _, err := os.Stat(vadConfig.ModelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("VAD model not found: %s", vadConfig.ModelPath)
	}

	// Create VAD
	vadModelConfig := sherpa.VadModelConfig{
		SileroVad: sherpa.SileroVadModelConfig{
			Model:             vadConfig.ModelPath,
			Threshold:         vadConfig.Threshold,
			MinSilenceDuration: vadConfig.MinSilenceDuration,
			MinSpeechDuration:  vadConfig.MinSpeechDuration,
			WindowSize:        512,
		},
		SampleRate: r.config.SampleRate,
		NumThreads: 1,
		Debug:      0,
	}

	vad := sherpa.NewVoiceActivityDetector(&vadModelConfig, 30) // 30 seconds buffer
	if vad == nil {
		return nil, fmt.Errorf("failed to create VAD")
	}
	defer sherpa.DeleteVoiceActivityDetector(vad)

	// Start ffmpeg to convert to raw PCM
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
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Process audio through VAD
	reader := bufio.NewReader(stdout)
	windowSize := 512
	windowBytes := windowSize * 2 // 16-bit = 2 bytes per sample

	var processedSamples int64
	var allTokens []Token
	var allText string

	reportProgress := func(step string) {
		if onProgress != nil {
			progressSec := float64(processedSamples) / float64(r.config.SampleRate)
			progress := int(30 + 60*progressSec/duration)
			if progress > 90 {
				progress = 90
			}
			onProgress(progress, step)
		}
	}

	reportProgress("transcribing")

	for {
		buffer := make([]byte, windowBytes)
		n, err := io.ReadFull(reader, buffer)

		if n == 0 {
			break
		}

		samples := bytesToFloat32(buffer[:n])
		vad.AcceptWaveform(samples)
		processedSamples += int64(len(samples))

		// Process detected speech segments
		for !vad.IsEmpty() {
			segment := vad.Front()
			vad.Pop()

			segmentStartSec := float32(segment.Start) / float32(r.config.SampleRate)

			// Transcribe this segment
			result, err := r.TranscribeBytes(segment.Samples, r.config.SampleRate)
			if err != nil {
				continue
			}

			// Adjust token timestamps with segment offset
			for _, token := range result.Tokens {
				allTokens = append(allTokens, Token{
					Text:      token.Text,
					StartTime: token.StartTime + segmentStartSec,
					Duration:  token.Duration,
				})
			}
			allText += result.Text
		}

		// Report progress periodically
		if int(processedSamples)%(r.config.SampleRate*2) == 0 {
			reportProgress("transcribing")
		}

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}

	// Flush remaining
	vad.Flush()
	for !vad.IsEmpty() {
		segment := vad.Front()
		vad.Pop()

		segmentStartSec := float32(segment.Start) / float32(r.config.SampleRate)

		result, err := r.TranscribeBytes(segment.Samples, r.config.SampleRate)
		if err != nil {
			continue
		}

		for _, token := range result.Tokens {
			allTokens = append(allTokens, Token{
				Text:      token.Text,
				StartTime: token.StartTime + segmentStartSec,
				Duration:  token.Duration,
			})
		}
		allText += result.Text
	}

	cmd.Wait()

	// Calculate total duration from last token
	var totalDuration float32
	if len(allTokens) > 0 {
		lastToken := allTokens[len(allTokens)-1]
		totalDuration = lastToken.StartTime + lastToken.Duration
	}

	return &Result{
		Text:          allText,
		Tokens:        allTokens,
		Segments:      tokensToSegments(allTokens),
		TotalDuration: totalDuration,
	}, nil
}

// bytesToFloat32 converts 16-bit PCM bytes to float32 samples
func bytesToFloat32(data []byte) []float32 {
	samples := make([]float32, len(data)/2)
	for i := 0; i < len(samples); i++ {
		sample := int16(binary.LittleEndian.Uint16(data[i*2:]))
		samples[i] = float32(sample) / 32768.0
	}
	return samples
}
