package asr

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// TranscribeWithTempo transcribes audio with optional tempo adjustment for fast speech
// Uses chunk-based processing (no VAD) for maximum accuracy
func (r *Recognizer) TranscribeWithTempo(inputPath string, tempo float64, chunkSec int, onProgress ProgressCallback) (*Result, error) {
	// Default values
	if tempo <= 0 {
		tempo = 1.0
	}
	if chunkSec <= 0 {
		chunkSec = 20
	}

	// Get audio duration for progress calculation
	duration, err := GetAudioDuration(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get audio duration: %w", err)
	}

	// Tempo correction factor
	// If tempo=0.95, audio is slower, so timestamps in slowed audio need to be multiplied by 0.95
	// to get original audio timestamps
	tempoFactor := tempo

	// Start ffmpeg with optional tempo adjustment
	var cmd *exec.Cmd
	if tempo != 1.0 {
		cmd = exec.Command("ffmpeg",
			"-i", inputPath,
			"-af", fmt.Sprintf("atempo=%.2f", tempo),
			"-f", "s16le",
			"-acodec", "pcm_s16le",
			"-ar", fmt.Sprintf("%d", r.config.SampleRate),
			"-ac", "1",
			"-loglevel", "error",
			"pipe:1",
		)
	} else {
		cmd = exec.Command("ffmpeg",
			"-i", inputPath,
			"-f", "s16le",
			"-acodec", "pcm_s16le",
			"-ar", fmt.Sprintf("%d", r.config.SampleRate),
			"-ac", "1",
			"-loglevel", "error",
			"pipe:1",
		)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Process in chunks
	reader := bufio.NewReader(stdout)
	chunkSamples := r.config.SampleRate * chunkSec
	chunkBytes := chunkSamples * 2 // 16-bit PCM

	var allTokens []Token
	var allText string
	var processedSamples int64
	chunkNum := 0

	reportProgress := func(step string) {
		if onProgress != nil {
			progressSec := float64(processedSamples) / float64(r.config.SampleRate) * tempoFactor
			progress := int(30 + 60*progressSec/duration)
			if progress > 90 {
				progress = 90
			}
			onProgress(progress, step)
		}
	}

	reportProgress("transcribing")

	for {
		buffer := make([]byte, chunkBytes)
		n, err := io.ReadFull(reader, buffer)
		if n == 0 {
			break
		}

		samples := bytesToFloat32Tempo(buffer[:n])
		processedSamples += int64(len(samples))
		chunkNum++

		// Raw time in slowed audio
		rawStartSec := float64(chunkNum-1) * float64(chunkSec)

		// Corrected time in original audio
		startSec := rawStartSec * tempoFactor

		// Transcribe chunk
		result, transcribeErr := r.TranscribeBytes(samples, r.config.SampleRate)
		if transcribeErr != nil {
			continue
		}

		// Adjust token timestamps
		for _, token := range result.Tokens {
			allTokens = append(allTokens, Token{
				Text:      token.Text,
				StartTime: float32(startSec) + token.StartTime*float32(tempoFactor),
				Duration:  token.Duration * float32(tempoFactor),
			})
		}
		allText += result.Text

		reportProgress("transcribing")

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
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

// bytesToFloat32Tempo converts 16-bit PCM bytes to float32 samples
func bytesToFloat32Tempo(data []byte) []float32 {
	samples := make([]float32, len(data)/2)
	for i := 0; i < len(samples); i++ {
		sample := int16(binary.LittleEndian.Uint16(data[i*2:]))
		samples[i] = float32(sample) / 32768.0
	}
	return samples
}
