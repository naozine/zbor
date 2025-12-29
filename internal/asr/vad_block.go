package asr

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// SpeechBlock represents a detected speech segment
type SpeechBlock struct {
	StartTime float64 // Start time in seconds
	EndTime   float64 // End time in seconds
}

// splitLongBlocks splits blocks longer than maxDuration into smaller chunks
func splitLongBlocks(blocks []SpeechBlock, maxDuration float64) []SpeechBlock {
	if maxDuration <= 0 {
		return blocks
	}

	var result []SpeechBlock
	for _, block := range blocks {
		duration := block.EndTime - block.StartTime
		if duration <= maxDuration {
			result = append(result, block)
			continue
		}

		// Split into chunks of maxDuration
		start := block.StartTime
		for start < block.EndTime {
			end := start + maxDuration
			if end > block.EndTime {
				end = block.EndTime
			}
			result = append(result, SpeechBlock{
				StartTime: start,
				EndTime:   end,
			})
			start = end
		}
	}
	return result
}

// TranscribeWithVADBlock transcribes audio using VAD to detect speech blocks,
// then processes each block with optional tempo adjustment.
// This approach avoids chunk boundary issues by processing natural speech units.
//
// 【本番用メソッド】
// 他のメソッド（TranscribeWithVAD, TranscribeWithTempo）は実験用。
//
// 推奨パラメータ:
//
//	vadConfig.Threshold = 0.1           // 感度を上げて小さい声も検出
//	vadConfig.MinSilenceDuration = 6.0  // ブロックをマージして小さい音声も含める
//	vadConfig.MaxBlockDuration = 5.0    // 長いブロックを5秒で分割（冒頭ドロップ防止）
//	tempo = 1.0                         // 通常は速度調整不要
//	config.DecodingMethod = ""          // greedy_search（beam_searchは不要）
func (r *Recognizer) TranscribeWithVADBlock(inputPath string, vadConfig *VADConfig, tempo float64, onProgress ProgressCallback) (*Result, error) {
	if tempo <= 0 {
		tempo = 1.0
	}

	// Step 1: Detect speech blocks using VAD
	if onProgress != nil {
		onProgress(10, "detecting speech")
	}

	blocks, err := r.detectSpeechBlocks(inputPath, vadConfig)
	if err != nil {
		return nil, fmt.Errorf("VAD detection failed: %w", err)
	}

	if len(blocks) == 0 {
		return &Result{
			Text:     "",
			Tokens:   []Token{},
			Segments: []Segment{},
		}, nil
	}

	// Split long blocks to avoid recognition dropping beginning of audio
	blocks = splitLongBlocks(blocks, vadConfig.MaxBlockDuration)

	// Debug: print detected blocks
	for i, b := range blocks {
		fmt.Fprintf(os.Stderr, "  Block %d: %.2f - %.2f (%.2fs)\n", i+1, b.StartTime, b.EndTime, b.EndTime-b.StartTime)
	}

	if onProgress != nil {
		onProgress(20, fmt.Sprintf("found %d blocks", len(blocks)))
	}

	// Step 2: Process each block
	var allTokens []Token
	var allText string

	for i, block := range blocks {
		if onProgress != nil {
			progress := 20 + int(60*float64(i)/float64(len(blocks)))
			onProgress(progress, fmt.Sprintf("transcribing block %d/%d", i+1, len(blocks)))
		}

		tokens, text, err := r.transcribeBlock(inputPath, block, tempo)
		if err != nil {
			// Log but continue with other blocks
			fmt.Fprintf(os.Stderr, "Warning: failed to transcribe block %d: %v\n", i+1, err)
			continue
		}

		allTokens = append(allTokens, tokens...)
		allText += text
	}

	// Sort tokens by start time (should already be sorted, but ensure)
	sort.Slice(allTokens, func(i, j int) bool {
		return allTokens[i].StartTime < allTokens[j].StartTime
	})

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
		Text:          allText,
		Tokens:        allTokens,
		Segments:      tokensToSegments(allTokens),
		TotalDuration: totalDuration,
	}, nil
}

// detectSpeechBlocks uses VAD to detect speech segments in the audio
func (r *Recognizer) detectSpeechBlocks(inputPath string, vadConfig *VADConfig) ([]SpeechBlock, error) {
	// Check VAD model exists
	if _, err := os.Stat(vadConfig.ModelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("VAD model not found: %s", vadConfig.ModelPath)
	}

	// Create VAD
	vadModelConfig := sherpa.VadModelConfig{
		SileroVad: sherpa.SileroVadModelConfig{
			Model:              vadConfig.ModelPath,
			Threshold:         vadConfig.Threshold,
			MinSilenceDuration: vadConfig.MinSilenceDuration,
			MinSpeechDuration:  vadConfig.MinSpeechDuration,
			WindowSize:        512,
		},
		SampleRate: r.config.SampleRate,
		NumThreads: 1,
		Debug:      0,
	}

	vad := sherpa.NewVoiceActivityDetector(&vadModelConfig, 60) // 60 seconds buffer
	if vad == nil {
		return nil, fmt.Errorf("failed to create VAD")
	}
	defer sherpa.DeleteVoiceActivityDetector(vad)

	// Convert audio to raw PCM (no tempo adjustment for VAD)
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
	windowBytes := windowSize * 2

	var blocks []SpeechBlock

	for {
		buffer := make([]byte, windowBytes)
		n, err := io.ReadFull(reader, buffer)
		if n == 0 {
			break
		}

		samples := bytesToFloat32(buffer[:n])
		vad.AcceptWaveform(samples)

		// Collect detected segments
		for !vad.IsEmpty() {
			segment := vad.Front()
			vad.Pop()

			startSec := float64(segment.Start) / float64(r.config.SampleRate)
			endSec := startSec + float64(len(segment.Samples))/float64(r.config.SampleRate)

			blocks = append(blocks, SpeechBlock{
				StartTime: startSec,
				EndTime:   endSec,
			})
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

		startSec := float64(segment.Start) / float64(r.config.SampleRate)
		endSec := startSec + float64(len(segment.Samples))/float64(r.config.SampleRate)

		blocks = append(blocks, SpeechBlock{
			StartTime: startSec,
			EndTime:   endSec,
		})
	}

	cmd.Wait()

	return blocks, nil
}

// transcribeBlock transcribes a single speech block with tempo adjustment
func (r *Recognizer) transcribeBlock(inputPath string, block SpeechBlock, tempo float64) ([]Token, string, error) {
	duration := block.EndTime - block.StartTime
	if duration <= 0 {
		return nil, "", nil
	}

	// Minimum duration check: blocks shorter than 0.1s (after tempo) cause ONNX model crash
	// Error: "Invalid input shape" when audio is too short for Conv layer
	// Note: tempo < 1.0 slows audio down, so resulting duration = duration / tempo
	minDuration := 0.1 // seconds
	resultingDuration := duration / tempo
	if resultingDuration < minDuration {
		// Skip blocks that are too short to process
		return nil, "", nil
	}

	// Use ffmpeg to extract block with tempo adjustment
	// Note: -ss and -t before -i applies to input (faster seek, duration is input duration)
	// This ensures tempo filter doesn't get truncated by -t
	args := []string{
		"-ss", fmt.Sprintf("%.3f", block.StartTime),
		"-t", fmt.Sprintf("%.3f", duration),
		"-i", inputPath,
	}

	// Add tempo filter if not 1.0
	if tempo != 1.0 {
		args = append(args, "-af", fmt.Sprintf("atempo=%.2f", tempo))
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
		return nil, "", fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Read all samples
	var allSamples []float32
	reader := bufio.NewReader(stdout)
	buf := make([]byte, 16000) // Read in larger chunks

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			samples := bytesToFloat32(buf[:n])
			allSamples = append(allSamples, samples...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			cmd.Wait()
			return nil, "", fmt.Errorf("failed to read audio: %w", err)
		}
	}

	cmd.Wait()

	if len(allSamples) == 0 {
		return nil, "", nil
	}

	// Transcribe
	result, err := r.TranscribeBytes(allSamples, r.config.SampleRate)
	if err != nil {
		return nil, "", fmt.Errorf("transcription failed: %w", err)
	}

	// Adjust timestamps to original audio time
	var adjustedTokens []Token
	for _, token := range result.Tokens {
		// Token timestamp is in slowed audio time, convert to original time
		adjustedTokens = append(adjustedTokens, Token{
			Text:      token.Text,
			StartTime: float32(block.StartTime + float64(token.StartTime)*tempo),
			Duration:  token.Duration * float32(tempo),
		})
	}

	return adjustedTokens, result.Text, nil
}
